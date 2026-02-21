import { getGlobalApiKey } from './api';

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/ui/v1';

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const headers = new Headers(options?.headers);
  const apiKey = getGlobalApiKey();
  if (apiKey) headers.set('X-API-Key', apiKey);
  headers.set('Content-Type', 'application/json');

  const res = await fetch(`${API_BASE_URL}${url}`, { ...options, headers });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: `HTTP ${res.status}` }));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  return res.json() as Promise<T>;
}

// -- Response types matching Go backend /api/ui/v1/agents --

export interface AgentStartResponse {
  agent_id: string;
  status: string;
  pid: number;
  port: number;
  started_at: string;
  log_file: string;
  message: string;
  endpoints: {
    health: string;
    reasoners: string;
    skills: string;
  };
}

export interface AgentStopResponse {
  agent_id: string;
  status: string;
  message: string;
}

export interface AgentStatusResponse {
  agent_id: string;
  name: string;
  is_running: boolean;
  status: string;
  pid: number;
  port: number;
  uptime: string;
  last_seen: string;
  configuration_required: boolean;
  configuration_status: string;
  endpoints?: {
    health: string;
    reasoners: string;
    skills: string;
  };
}

export interface RunningAgentsResponse {
  running_agents: Array<{
    agent_id: string;
    name: string;
    status: string;
    pid: number;
    port: number;
    started_at: string;
    log_file: string;
    package?: {
      name: string;
      version: string;
      description: string;
      author: string;
    };
    endpoints: {
      health: string;
      reasoners: string;
      skills: string;
    };
  }>;
  total_count: number;
}

// -- API calls --

export function startAgent(agentId: string, opts?: { port?: number; detach?: boolean }): Promise<AgentStartResponse> {
  return request(`/agents/${agentId}/start`, {
    method: 'POST',
    body: JSON.stringify({ port: opts?.port ?? 0, detach: opts?.detach ?? true }),
  });
}

export function stopAgent(agentId: string): Promise<AgentStopResponse> {
  return request(`/agents/${agentId}/stop`, { method: 'POST' });
}

export function getAgentStatus(agentId: string): Promise<AgentStatusResponse> {
  return request(`/agents/${agentId}/status`);
}

export function listRunningAgents(): Promise<RunningAgentsResponse> {
  return request('/agents/running');
}

export function reconcileAgent(agentId: string): Promise<AgentStatusResponse> {
  return request(`/agents/${agentId}/reconcile`, { method: 'POST' });
}
