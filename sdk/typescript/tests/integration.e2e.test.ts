import express from 'express';
import http from 'node:http';
import axios from 'axios';
import { WebSocketServer } from 'ws';
import { beforeAll, afterAll, describe, expect, it } from 'vitest';
import { Agent } from '../src/agent/Agent.js';
import { HanzoAgentsClient } from '../src/client/HanzoAgentsClient.js';

type MemoryEntry = { key: string; value: any; scope: string; scopeId?: string };
type VectorEntry = { key: string; embedding: number[]; scope: string; scopeId?: string };

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

async function waitFor(predicate: () => boolean, timeoutMs = 3000, intervalMs = 25) {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    if (predicate()) return;
    await sleep(intervalMs);
  }
  throw new Error('Timed out waiting for condition');
}

async function getFreePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = http.createServer();
    server.listen(0, () => {
      const address = server.address();
      if (address && typeof address === 'object') {
        const port = address.port;
        server.close(() => resolve(port));
      } else {
        reject(new Error('Failed to acquire test port'));
      }
    });
    server.on('error', reject);
  });
}

function resolveScopeId(scope: string | undefined, headers: http.IncomingHttpHeaders) {
  const pick = (key: string) => {
    const value = headers[key];
    return Array.isArray(value) ? value[0] : value;
  };

  if (scope === 'session') return pick('x-session-id');
  if (scope === 'actor') return pick('x-actor-id');
  if (scope === 'global') return 'global';
  return pick('x-workflow-id') ?? pick('x-run-id');
}

function forwardMetadataHeaders(headers: http.IncomingHttpHeaders) {
  const allowed = [
    'x-run-id',
    'x-workflow-id',
    'x-session-id',
    'x-actor-id',
    'x-execution-id',
    'x-parent-execution-id',
    'x-caller-did',
    'x-target-did',
    'x-agent-node-did',
    'x-agent-node-id'
  ];
  const forwarded: Record<string, string> = {};
  for (const key of allowed) {
    const value = headers[key];
    if (value !== undefined) {
      forwarded[key] = Array.isArray(value) ? value[0] : String(value);
    }
  }
  return forwarded;
}

async function createControlPlaneStub() {
  const app = express();
  app.use(express.json());

  const registrations: any[] = [];
  const heartbeats: Array<{ nodeId: string; status?: string }> = [];
  const workflowEvents: any[] = [];
  const executionStatuses: any[] = [];
  const memory: MemoryEntry[] = [];
  const vectors: VectorEntry[] = [];
  const agents = new Map<string, { baseUrl: string; reasoners: any[]; skills: any[] }>();

  const server = http.createServer(app);
  const wss = new WebSocketServer({ server, path: '/api/v1/memory/events/ws' });
  const sockets = new Set<any>();

  wss.on('connection', (socket) => {
    sockets.add(socket);
    socket.on('close', () => sockets.delete(socket));
  });

  const findMemory = (scope: string, scopeId: string | undefined, key: string) =>
    memory.find((entry) => entry.scope === scope && entry.scopeId === scopeId && entry.key === key);

  app.post('/api/v1/nodes/register', (req, res) => {
    const payload = req.body ?? {};
    registrations.push(payload);
    agents.set(payload.id, {
      baseUrl: payload.base_url ?? payload.public_url,
      reasoners: payload.reasoners ?? [],
      skills: payload.skills ?? []
    });
    res.json({ ok: true });
  });

  app.post('/api/v1/nodes/:id/heartbeat', (req, res) => {
    heartbeats.push({ nodeId: req.params.id, status: req.body?.status });
    res.json({ status: req.body?.status ?? 'ready', nodeId: req.params.id });
  });

  app.post('/api/v1/workflow/executions/events', (req, res) => {
    workflowEvents.push(req.body);
    res.json({ ok: true });
  });

  app.post('/api/v1/executions/:id/status', (req, res) => {
    executionStatuses.push({ id: req.params.id, body: req.body });
    res.json({ ok: true });
  });

  app.get('/api/v1/discovery/capabilities', (_req, res) => {
    const capabilities = Array.from(agents.entries()).map(([agentId, info]) => ({
      agent_id: agentId,
      base_url: info.baseUrl,
      version: '',
      health_status: 'running',
      deployment_type: 'long_running',
      last_heartbeat: heartbeats.find((hb) => hb.nodeId === agentId)?.status,
      reasoners: info.reasoners.map((r: any) => ({
        id: r.id,
        invocation_target: `${agentId}.${r.id}`,
        tags: r.tags ?? [],
        input_schema: r.input_schema ?? {},
        output_schema: r.output_schema ?? {}
      })),
      skills: info.skills.map((s: any) => ({
        id: s.id,
        invocation_target: `${agentId}.${s.id}`,
        tags: s.tags ?? [],
        input_schema: s.input_schema ?? {}
      }))
    }));

    const totalReasoners = capabilities.reduce((total, cap) => total + cap.reasoners.length, 0);
    const totalSkills = capabilities.reduce((total, cap) => total + cap.skills.length, 0);

    res.json({
      discovered_at: new Date().toISOString(),
      total_agents: capabilities.length,
      total_reasoners: totalReasoners,
      total_skills: totalSkills,
      pagination: { limit: capabilities.length, offset: 0, has_more: false },
      capabilities
    });
  });

  app.post('/api/v1/execute/:target', async (req, res) => {
    const rawTarget = req.params.target;
    const [agentIdMaybe, nameMaybe] = rawTarget.includes('.') ? rawTarget.split('.', 2) : [undefined, rawTarget];
    const agentId = agentIdMaybe ?? registrations.at(-1)?.id;
    const name = nameMaybe ?? rawTarget;
    const agentInfo = agentId ? agents.get(agentId) : undefined;

    if (!agentInfo) {
      res.status(404).json({ error: 'Agent not registered' });
      return;
    }

    const targetType = agentInfo.skills.some((s) => s.id === name) ? 'skill' : 'reasoner';
    const path = targetType === 'skill' ? `/api/v1/skills/${name}` : `/api/v1/reasoners/${name}`;

    try {
      const response = await axios.post(`${agentInfo.baseUrl}${path}`, req.body?.input ?? {}, {
        headers: forwardMetadataHeaders(req.headers)
      });
      res.json({ result: response.data });
    } catch (err: any) {
      res.status(err?.response?.status ?? 500).json(err?.response?.data ?? { error: 'Forward failed' });
    }
  });

  app.post('/api/v1/memory/set', (req, res) => {
    const scope = req.body?.scope ?? 'workflow';
    const scopeId = resolveScopeId(scope, req.headers);
    const existing = findMemory(scope, scopeId, req.body?.key);
    if (existing) {
      existing.value = req.body?.data;
    } else {
      memory.push({ key: req.body?.key, value: req.body?.data, scope, scopeId });
    }
    res.json({ ok: true });
  });

  app.post('/api/v1/memory/get', (req, res) => {
    const scope = req.body?.scope ?? 'workflow';
    const scopeId = resolveScopeId(scope, req.headers);
    const entry = findMemory(scope, scopeId, req.body?.key);
    if (!entry) {
      res.status(404).json({ error: 'not found' });
      return;
    }
    res.json({ data: entry.value });
  });

  app.post('/api/v1/memory/delete', (req, res) => {
    const scope = req.body?.scope ?? 'workflow';
    const scopeId = resolveScopeId(scope, req.headers);
    const idx = memory.findIndex(
      (entry) => entry.scope === scope && entry.scopeId === scopeId && entry.key === req.body?.key
    );
    if (idx >= 0) memory.splice(idx, 1);
    res.json({ ok: true });
  });

  app.get('/api/v1/memory/list', (req, res) => {
    const scope = String(req.query.scope ?? 'workflow');
    res.json(memory.filter((entry) => entry.scope === scope).map((entry) => ({ key: entry.key })));
  });

  app.post('/api/v1/memory/vector/set', (req, res) => {
    const scope = req.body?.scope ?? 'workflow';
    const scopeId = resolveScopeId(scope, req.headers);
    const existing = vectors.find(
      (entry) => entry.scope === scope && entry.scopeId === scopeId && entry.key === req.body?.key
    );
    if (existing) {
      existing.embedding = req.body?.embedding ?? [];
    } else {
      vectors.push({
        key: req.body?.key,
        embedding: req.body?.embedding ?? [],
        scope,
        scopeId
      });
    }
    res.json({ ok: true });
  });

  app.post('/api/v1/memory/vector/search', (req, res) => {
    const scope = req.body?.scope ?? 'workflow';
    const scopeId = resolveScopeId(scope, req.headers);
    const matches = vectors
      .filter((entry) => entry.scope === scope && entry.scopeId === scopeId)
      .slice(0, req.body?.top_k ?? 10)
      .map((entry) => ({
        key: entry.key,
        scope: entry.scope,
        scopeId: entry.scopeId ?? '',
        score: 1.0
      }));
    res.json(matches);
  });

  app.post('/api/v1/memory/vector/delete', (req, res) => {
    const scope = req.body?.scope ?? 'workflow';
    const scopeId = resolveScopeId(scope, req.headers);
    const idx = vectors.findIndex(
      (entry) => entry.scope === scope && entry.scopeId === scopeId && entry.key === req.body?.key
    );
    if (idx >= 0) vectors.splice(idx, 1);
    res.json({ ok: true });
  });

  const port = await getFreePort();
  await new Promise<void>((resolve) => server.listen(port, '127.0.0.1', () => resolve()));

  return {
    url: `http://127.0.0.1:${port}`,
    registrations,
    heartbeats,
    workflowEvents,
    executionStatuses,
    memory,
    stop: async () => {
      sockets.forEach((socket) => socket.close());
      await new Promise<void>((resolve) => wss.close(() => resolve()));
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  };
}

describe('TypeScript SDK integration', () => {
  let control: Awaited<ReturnType<typeof createControlPlaneStub>>;
  let agent: Agent;
  let client: HanzoAgentsClient;
  let agentPort: number;

  beforeAll(async () => {
    control = await createControlPlaneStub();
    agentPort = await getFreePort();

    agent = new Agent({
      nodeId: 'ts-e2e-agent',
      port: agentPort,
      host: '127.0.0.1',
      controlPlaneUrl: control.url,
      heartbeatIntervalMs: 20,
      devMode: false
    });

    agent.reasoner('echo', async (ctx) => {
      await ctx.memory.set('last_input', ctx.input.message);
      const stored = await ctx.memory.get('last_input');
      return {
        echoed: ctx.input.message,
        stored,
        workflowId: ctx.workflowId,
        runId: ctx.runId
      };
    });

    agent.skill('greet', (ctx) => ({ greeting: `hello ${ctx.input.name}` }));

    await agent.serve();
    client = new HanzoAgentsClient({ nodeId: 'ts-e2e-client', controlPlaneUrl: control.url });
  }, 20000);

  afterAll(async () => {
    await agent.shutdown();
    await control.stop();
  });

  it('registers with the control plane and surfaces capabilities', async () => {
    await waitFor(() => control.registrations.length > 0);

    const registration = control.registrations.at(-1);
    expect(registration.reasoners.map((r: any) => r.id)).toContain('echo');
    expect(control.heartbeats.some((hb) => hb.status === 'starting')).toBe(true);

    const discovery = await client.discoverCapabilities({ agent: 'ts-e2e-agent' });
    const capability = discovery.json?.capabilities.find((cap) => cap.agentId === 'ts-e2e-agent');

    expect(capability?.reasoners.map((r) => r.id)).toContain('echo');
    expect(capability?.skills.map((s) => s.id)).toContain('greet');
  });

  it('executes through the control plane and persists memory', async () => {
    const result = await client.execute<{ echoed: string; stored: string; workflowId: string; runId: string }>(
      'ts-e2e-agent.echo',
      { message: 'integration-hello' },
      { runId: 'run-42', workflowId: 'wf-42' }
    );

    expect(result.echoed).toBe('integration-hello');
    expect(result.stored).toBe('integration-hello');
    expect(result.workflowId).toBe('wf-42');

    const stored = control.memory.find(
      (entry) => entry.key === 'last_input' && entry.scope === 'workflow' && entry.scopeId === 'wf-42'
    );
    expect(stored?.value).toBe('integration-hello');
  });

  it('publishes workflow events for local agent.call executions', async () => {
    const response = await agent.call('ts-e2e-agent.echo', { message: 'local-hop' });
    expect(response.echoed).toBe('local-hop');

    await waitFor(() => control.workflowEvents.length >= 2);
    const lastEvents = control.workflowEvents.slice(-2);

    expect(lastEvents[0].status).toBe('running');
    expect(lastEvents[1].status).toBe('succeeded');
    expect(lastEvents[1].reasoner_id).toBe('echo');
    expect(lastEvents[1].agent_node_id).toBe('ts-e2e-agent');
  });
});
