import type { Agent } from '../agent/Agent.js';
import type { MCPServerConfig } from '../types/agent.js';
import type { MCPTool, MCPToolRegistration } from '../types/mcp.js';
import { MCPClientRegistry } from './MCPClientRegistry.js';

export interface MCPToolRegistrarOptions {
  namespace?: string;
  tags?: string[];
  devMode?: boolean;
}

export class MCPToolRegistrar {
  private readonly registered = new Set<string>();
  private readonly devMode: boolean;

  constructor(
    private readonly agent: Agent,
    private readonly registry: MCPClientRegistry,
    private readonly options: MCPToolRegistrarOptions = {}
  ) {
    this.devMode = Boolean(options.devMode);
  }

  registerServers(servers: MCPServerConfig[]) {
    servers.forEach((server) => this.registry.register(server));
  }

  async registerAll(): Promise<{ registered: MCPToolRegistration[] }> {
    const registrations: MCPToolRegistration[] = [];
    const clients = this.registry.list();

    for (const client of clients) {
      const healthy = await client.healthCheck();
      if (!healthy) {
        if (this.devMode) {
          console.warn(`Skipping MCP server ${client.alias} (health check failed)`);
        }
        continue;
      }

      const tools = await client.listTools();
      for (const tool of tools) {
        if (!tool?.name) continue;

        const skillName = this.buildSkillName(client.alias, tool.name);
        if (this.registered.has(skillName) || this.agent.skills.get(skillName)) {
          continue;
        }

        this.agent.skill(
          skillName,
          async (ctx) => {
            const args = (ctx.input && typeof ctx.input === 'object') ? (ctx.input as Record<string, any>) : {};
            const result = await client.callTool(tool.name, args);
            return {
              status: 'success',
              result,
              server: client.alias,
              tool: tool.name
            };
          },
          {
            description: tool.description ?? `MCP tool ${tool.name} from ${client.alias}`,
            inputSchema: tool.inputSchema ?? tool.input_schema ?? {},
            tags: this.buildTags(client.alias)
          }
        );

        this.registered.add(skillName);
        registrations.push({ server: client.alias, skillName, tool });
        if (this.devMode) {
          console.info(`Registered MCP skill ${skillName}`);
        }
      }
    }

    return { registered: registrations };
  }

  private buildTags(alias: string) {
    return Array.from(new Set(['mcp', alias, ...(this.options.tags ?? [])]));
  }

  private buildSkillName(serverAlias: string, toolName: string) {
    const base = [this.options.namespace, serverAlias, toolName].filter(Boolean).join('_');
    return this.sanitize(base);
  }

  private sanitize(value: string) {
    const collapsed = value.replace(/[^a-zA-Z0-9_]/g, '_').replace(/_+/g, '_').replace(/^_+|_+$/g, '');
    if (/^[0-9]/.test(collapsed)) {
      return `mcp_${collapsed}`;
    }
    return collapsed || 'mcp_tool';
  }
}
