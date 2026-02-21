import { getGlobalApiKey } from './api';

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/ui/v1';
const GATEWAY_URL = import.meta.env.VITE_GATEWAY_URL || 'wss://gateway.hanzo.bot';

async function fetchWithAuth<T>(url: string, options?: RequestInit): Promise<T> {
  const headers = new Headers(options?.headers || {});
  const apiKey = getGlobalApiKey();
  if (apiKey) {
    headers.set('X-API-Key', apiKey);
  }
  headers.set('Content-Type', 'application/json');

  const response = await fetch(`${API_BASE_URL}${url}`, {
    ...options,
    headers,
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: `HTTP ${response.status}` }));
    throw new Error(error.message || `Request failed: ${response.status}`);
  }

  return response.json();
}

// Team presets
export async function getTeamPresets() {
  return fetchWithAuth<{ presets: unknown[] }>('/agents/presets');
}

// Agent instances
export async function listAgents() {
  return fetchWithAuth<{ agents: unknown[] }>('/agents');
}

export async function createAgent(data: { presetId: string; name: string; config?: Record<string, unknown> }) {
  return fetchWithAuth<{ agent: unknown }>('/agents', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function deleteAgent(agentId: string) {
  return fetchWithAuth<{ success: boolean }>(`/agents/${agentId}`, {
    method: 'DELETE',
  });
}

export async function startAgent(agentId: string) {
  return fetchWithAuth<{ agent: unknown }>(`/agents/${agentId}/start`, {
    method: 'POST',
  });
}

export async function stopAgent(agentId: string) {
  return fetchWithAuth<{ agent: unknown }>(`/agents/${agentId}/stop`, {
    method: 'POST',
  });
}

// Agent chat (via bot gateway WebSocket)
export function connectToAgent(agentId: string, token?: string): WebSocket {
  const url = new URL(GATEWAY_URL);
  if (token) url.searchParams.set('token', token);
  url.searchParams.set('agent', agentId);
  return new WebSocket(url.toString());
}

// Agent DID
export async function getAgentDID(agentId: string) {
  return fetchWithAuth<{ did: string; document?: unknown }>(`/agents/${agentId}/did`);
}

// Agent wallet
export async function getAgentWallet(agentId: string) {
  return fetchWithAuth<{ address: string; balance?: string }>(`/agents/${agentId}/wallet`);
}

// Agent execution history
export async function getAgentExecutions(agentId: string, limit = 20) {
  return fetchWithAuth<{ executions: unknown[] }>(`/agents/${agentId}/executions?limit=${limit}`);
}

// Agent logs (SSE)
export function subscribeToAgentLogs(agentId: string): EventSource {
  const apiKey = getGlobalApiKey();
  const url = apiKey
    ? `${API_BASE_URL}/agents/${agentId}/logs?api_key=${encodeURIComponent(apiKey)}`
    : `${API_BASE_URL}/agents/${agentId}/logs`;
  return new EventSource(url);
}

// Dashboard summary
export async function getAgentDashboardSummary() {
  return fetchWithAuth<{
    totalAgents: number;
    activeAgents: number;
    totalExecutions: number;
    avgResponseTime: number;
  }>('/agents/summary');
}
