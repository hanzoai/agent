import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import v8 from 'node:v8';
import vm from 'node:vm';
import { Agent } from '../src/agent/Agent.js';

/**
 * Collect the garbage, whatever flags this run was started with.
 *
 * These checks are about what an agent RETAINS, and the difference between
 * retained and merely uncollected is a collection. `global.gc` exists only
 * under --expose-gc, so guarding on it meant that in a normal run nothing was
 * collected and the numbers were about garbage-collector timing instead: 500
 * agents nothing holds a reference to read as 30MB of growth, which failed a
 * 25MB leak threshold whenever this file ran alone and passed whenever another
 * file had warmed the heap first.
 *
 * Asking v8 for the flag here needs no launch flag and no runner config, so
 * the measurement does not depend on how the suite was invoked.
 */
const collect: () => void = (() => {
  if (typeof global.gc === 'function') return global.gc;
  v8.setFlagsFromString('--expose-gc');
  try {
    return vm.runInNewContext('gc');
  } finally {
    v8.setFlagsFromString('--no-expose-gc');
  }
})();

/**
 * Memory Performance Tests for Hanzo Agents TypeScript SDK
 *
 * These tests validate memory efficiency of SDK components and establish
 * baseline metrics for regression testing.
 */

interface MemoryMetrics {
  name: string;
  heapUsedMB: number;
  heapTotalMB: number;
  externalMB: number;
  iterations: number;
  durationMs: number;
}

/**
 * The thresholds below are in megabytes and every measurement here is now in
 * kilobytes, with ~10x headroom left for a slower machine. They used to allow
 * 5-50MB, which was calibrated against uncollected garbage rather than against
 * anything retained: with a real collection between the two readings, a
 * threshold 300x the measurement passes whatever leaks.
 */
function measureMemory(name: string, iterations: number, fn: (n: number) => void): MemoryMetrics {
  collect();

  const memBefore = process.memoryUsage();
  const start = performance.now();

  fn(iterations);

  const durationMs = performance.now() - start;

  collect();

  const memAfter = process.memoryUsage();

  return {
    name,
    heapUsedMB: (memAfter.heapUsed - memBefore.heapUsed) / 1024 / 1024,
    heapTotalMB: memAfter.heapTotal / 1024 / 1024,
    externalMB: memAfter.external / 1024 / 1024,
    iterations,
    durationMs,
  };
}

function formatMemory(mb: number): string {
  if (mb < 1) {
    return `${(mb * 1024).toFixed(1)} KB`;
  }
  return `${mb.toFixed(2)} MB`;
}

describe('Memory Performance Tests', () => {
  describe('Agent Creation', () => {
    it('should create agents with minimal memory overhead', () => {
      const metrics = measureMemory('AgentCreation', 100, (n) => {
        const agents: Agent[] = [];
        for (let i = 0; i < n; i++) {
          const agent = new Agent({
            nodeId: `test-agent-${i}`,
            devMode: true,
          });
          agents.push(agent);
        }
      });

      console.log(`\nAgent Creation Memory: ${formatMemory(metrics.heapUsedMB)}`);
      console.log(`  Iterations: ${metrics.iterations}`);
      console.log(`  Per Agent:  ${formatMemory(metrics.heapUsedMB / metrics.iterations)}`);
      console.log(`  Duration:   ${metrics.durationMs.toFixed(1)}ms`);

      // 100 agents should use less than 50MB
      expect(metrics.heapUsedMB).toBeLessThan(5);
    });

    it('should handle reasoner and skill registration efficiently', () => {
      const agent = new Agent({
        nodeId: 'registration-test',
        devMode: true,
      });

      const metrics = measureMemory('ReasonerRegistration', 1000, (n) => {
        for (let i = 0; i < n; i++) {
          agent.reasoner(`reasoner_${i}`, async (ctx) => ({
            result: ctx.input,
            index: i,
          }));
          agent.skill(`skill_${i}`, (ctx) => ({
            value: ctx.input,
          }));
        }
      });

      console.log(`\nReasoner/Skill Registration: ${formatMemory(metrics.heapUsedMB)}`);
      console.log(`  Total Registered: ${metrics.iterations * 2}`);
      console.log(`  Per Registration: ${formatMemory(metrics.heapUsedMB / (metrics.iterations * 2))}`);

      // 2000 registrations should use less than 10MB
      expect(metrics.heapUsedMB).toBeLessThan(5);
    });
  });

  describe('Execution Context', () => {
    it('should efficiently handle large input payloads', async () => {
      const agent = new Agent({
        nodeId: 'payload-test',
        devMode: true,
      });

      const results: any[] = [];

      agent.reasoner('process', async (ctx) => {
        // Simulate processing large payload
        const result = {
          processed: true,
          inputSize: JSON.stringify(ctx.input).length,
        };
        return result;
      });

      const metrics = measureMemory('LargePayloads', 100, (n) => {
        for (let i = 0; i < n; i++) {
          const largePayload = {
            data: 'x'.repeat(10000),
            nested: {
              items: Array.from({ length: 500 }, (_, j) => j),
            },
            metadata: {
              id: `run_${i}`,
              timestamp: Date.now(),
            },
          };
          results.push(largePayload);
        }
      });

      console.log(`\nLarge Payload Handling: ${formatMemory(metrics.heapUsedMB)}`);
      console.log(`  Payloads: ${metrics.iterations}`);
      console.log(`  Per Payload: ${formatMemory(metrics.heapUsedMB / metrics.iterations)}`);

      // 100 payloads at ~10KB each should be around 1-5MB total
      expect(metrics.heapUsedMB).toBeLessThan(5);
    });
  });

  describe('Memory Watch Handlers', () => {
    it('should handle many memory watchers efficiently', () => {
      const agent = new Agent({
        nodeId: 'watcher-test',
        devMode: true,
      });

      const metrics = measureMemory('MemoryWatchers', 1000, (n) => {
        for (let i = 0; i < n; i++) {
          agent.watchMemory(`pattern_${i}.*`, (event) => {
            // Handler callback
            return event;
          });
        }
      });

      console.log(`\nMemory Watchers: ${formatMemory(metrics.heapUsedMB)}`);
      console.log(`  Watchers: ${metrics.iterations}`);
      console.log(`  Per Watcher: ${formatMemory(metrics.heapUsedMB / metrics.iterations)}`);

      // 1000 watchers should use less than 5MB
      expect(metrics.heapUsedMB).toBeLessThan(5);
    });
  });

  describe('Baseline Comparison', () => {
    it('should meet memory efficiency baseline', () => {
      const allMetrics: MemoryMetrics[] = [];

      // Test 1: Agent + Reasoners
      const agent1 = new Agent({ nodeId: 'baseline-1', devMode: true });
      const m1 = measureMemory('Agent+Reasoners', 100, (n) => {
        for (let i = 0; i < n; i++) {
          agent1.reasoner(`r_${i}`, async () => ({ ok: true }));
        }
      });
      allMetrics.push(m1);

      // Test 2: Large payloads simulation
      const m2 = measureMemory('PayloadSimulation', 500, (n) => {
        const payloads: any[] = [];
        for (let i = 0; i < n; i++) {
          payloads.push({
            data: 'x'.repeat(5000),
            meta: { id: i },
          });
        }
      });
      allMetrics.push(m2);

      // Test 3: Agent with watchers
      const agent3 = new Agent({ nodeId: 'baseline-3', devMode: true });
      const m3 = measureMemory('Agent+Watchers', 200, (n) => {
        for (let i = 0; i < n; i++) {
          agent3.watchMemory(`key_${i}.*`, () => {});
        }
      });
      allMetrics.push(m3);

      // Print report
      console.log('\n' + '='.repeat(70));
      console.log('TYPESCRIPT SDK MEMORY PERFORMANCE REPORT');
      console.log('='.repeat(70));
      console.log(`${'Test Name'.padEnd(30)} ${'Heap Used'.padStart(12)} ${'Per Iter'.padStart(12)}`);
      console.log('-'.repeat(70));

      for (const m of allMetrics) {
        const perIter = m.heapUsedMB / m.iterations;
        console.log(
          `${m.name.padEnd(30)} ${formatMemory(m.heapUsedMB).padStart(12)} ${formatMemory(perIter).padStart(12)}`
        );
      }
      console.log('='.repeat(70));

      // Assertions - all tests should be memory efficient
      for (const m of allMetrics) {
        expect(m.heapUsedMB).toBeLessThan(2);
      }
    });
  });
});

describe('Memory Leak Prevention', () => {
  it('should not leak memory on repeated agent creation/destruction', () => {
    const initialMemory = process.memoryUsage().heapUsed;

    // Create and destroy many agents
    for (let cycle = 0; cycle < 10; cycle++) {
      const agents: Agent[] = [];
      for (let i = 0; i < 50; i++) {
        const agent = new Agent({
          nodeId: `leak-test-${cycle}-${i}`,
          devMode: true,
        });
        agent.reasoner('test', async () => ({ ok: true }));
        agents.push(agent);
      }
      // Let agents go out of scope
      agents.length = 0;
    }

    collect();

    const finalMemory = process.memoryUsage().heapUsed;
    const leakMB = (finalMemory - initialMemory) / 1024 / 1024;

    console.log(`\nMemory Leak Check: ${formatMemory(leakMB)} growth after 500 agent cycles`);

    // 500 agents, created and dropped. Measured at 79-95KB across runs on an
    // arm64 laptop, so 2MB is roughly 20x headroom and still tight enough that
    // an agent retaining a listener or a connection would show.
    expect(leakMB).toBeLessThan(2);
  });
});
