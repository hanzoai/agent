export interface MCPTool {
  name: string;
  description?: string;
  inputSchema?: any;
  input_schema?: any;
}

export interface MCPToolRegistration {
  server: string;
  skillName: string;
  tool: MCPTool;
}

export interface MCPHealthSummary {
  status: 'ok' | 'degraded' | 'disabled';
  totalServers: number;
  healthyServers: number;
  servers: Array<{
    alias: string;
    baseUrl: string;
    transport: 'http' | 'bridge';
    healthy: boolean;
  }>;
}
