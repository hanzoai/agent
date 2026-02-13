import express from 'express';
import type http from 'node:http';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Agent } from '../src/agent/Agent.js';
import { SkillContext } from '../src/context/SkillContext.js';

describe('MCP integration', () => {
  let server: http.Server;
  let baseUrl = '';

  beforeAll(async () => {
    const app = express();
    app.use(express.json());

    app.get('/health', (_req, res) => {
      res.json({ status: 'ok' });
    });

    app.post('/mcp/v1', (req, res) => {
      const method = req.body?.method;
      if (method === 'tools/list') {
        res.json({
          jsonrpc: '2.0',
          id: req.body?.id ?? 1,
          result: {
            tools: [
              {
                name: 'echo',
                description: 'Echo back the provided message',
                inputSchema: {
                  type: 'object',
                  properties: { message: { type: 'string' } },
                  required: ['message']
                }
              }
            ]
          }
        });
        return;
      }

      if (method === 'tools/call') {
        const message = req.body?.params?.arguments?.message;
        res.json({
          jsonrpc: '2.0',
          id: req.body?.id ?? 1,
          result: { echoed: message ?? null }
        });
        return;
      }

      res.status(400).json({ error: 'unknown method' });
    });

    await new Promise<void>((resolve) => {
      server = app.listen(0, () => {
        const port = (server.address() as any).port;
        baseUrl = `http://127.0.0.1:${port}`;
        resolve();
      });
    });
  });

  afterAll(async () => {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  });

  it('registers MCP tools as skills and executes them', async () => {
    const agent = new Agent({
      nodeId: 'mcp-agent',
      devMode: true,
      mcp: {
        servers: [{ alias: 'demo', url: baseUrl }],
        autoRegisterTools: true
      }
    });

    const { registered } = await agent.registerMcpTools();
    expect(registered.length).toBe(1);
    expect(registered[0]?.skillName).toBe('demo_echo');

    const skill = agent.skills.get('demo_echo');
    expect(skill).toBeDefined();

    const ctx = new SkillContext({
      input: { message: 'hello' },
      executionId: 'exec-1',
      sessionId: 'session-1',
      workflowId: 'wf-1',
      callerDid: undefined,
      agentNodeDid: undefined,
      req: {} as any,
      res: {} as any,
      agent,
      memory: agent.getMemoryInterface({ executionId: 'exec-1', runId: 'run-1', workflowId: 'wf-1' }),
      workflow: agent.getWorkflowReporter({ executionId: 'exec-1', runId: 'run-1', workflowId: 'wf-1' } as any),
      did: agent.getDidInterface({ executionId: 'exec-1', runId: 'run-1', workflowId: 'wf-1' } as any, { message: 'hello' })
    });

    const result = await skill!.handler(ctx as any);
    expect(result).toEqual({
      status: 'success',
      result: { echoed: 'hello' },
      server: 'demo',
      tool: 'echo'
    });
  });
});
