// Agent Canvas types for the ReactFlow-based agent control plane

export type AgentStatus = 'running' | 'idle' | 'error' | 'starting' | 'stopping'

export type AgentViewMode = 'chat' | 'terminal' | 'logs' | 'info'

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  timestamp: string
}

export interface LogEntry {
  id: string
  level: 'info' | 'warn' | 'error' | 'debug'
  message: string
  timestamp: string
  source?: string
}

export interface AgentStats {
  totalExecutions: number
  successCount: number
  failedCount: number
  avgResponseTimeMs: number
}

export interface AgentInstance {
  id: string
  name: string
  emoji: string
  role: string
  model: string
  status: AgentStatus
  did?: string
  wallet?: string
  stats: AgentStats
  messages: ChatMessage[]
  logs: LogEntry[]
  createdAt: string
  lastActive?: string
}

export interface AgentNodeData extends AgentInstance {
  viewMode: AgentViewMode
  isStreaming?: boolean
}
