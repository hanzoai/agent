import { createContext, useContext, useCallback, useState, type ReactNode } from 'react'
import type { AgentNodeData, AgentViewMode, ChatMessage } from '@/types/agent-canvas'
import * as lifecycle from '@/services/agentLifecycle'

interface AgentCanvasContextValue {
  /** Send a chat message to an agent */
  sendMessage: (agentId: string, content: string) => void
  /** Start an agent */
  startAgent: (agentId: string) => void
  /** Stop an agent */
  stopAgent: (agentId: string) => void
  /** Delete an agent from the canvas */
  deleteAgent: (agentId: string) => void
  /** Set the active view tab for an agent node */
  setViewMode: (agentId: string, mode: AgentViewMode) => void
  /** Map of agent ID to node data */
  agents: Record<string, AgentNodeData>
}

const AgentCanvasContext = createContext<AgentCanvasContextValue | null>(null)

export function useAgentCanvas(): AgentCanvasContextValue {
  const ctx = useContext(AgentCanvasContext)
  if (!ctx) {
    throw new Error('useAgentCanvas must be used within an AgentCanvasProvider')
  }
  return ctx
}

interface AgentCanvasProviderProps {
  children: ReactNode
  initialAgents?: Record<string, AgentNodeData>
}

export function AgentCanvasProvider({ children, initialAgents = {} }: AgentCanvasProviderProps) {
  const [agents, setAgents] = useState<Record<string, AgentNodeData>>(initialAgents)

  const sendMessage = useCallback((agentId: string, content: string) => {
    const msg: ChatMessage = {
      id: crypto.randomUUID(),
      role: 'user',
      content,
      timestamp: new Date().toISOString(),
    }
    setAgents((prev) => {
      const agent = prev[agentId]
      if (!agent) return prev
      return {
        ...prev,
        [agentId]: {
          ...agent,
          messages: [...agent.messages, msg],
          isStreaming: true,
        },
      }
    })
  }, [])

  const startAgent = useCallback((agentId: string) => {
    // Optimistic update
    setAgents((prev) => {
      const agent = prev[agentId]
      if (!agent) return prev
      return { ...prev, [agentId]: { ...agent, status: 'starting' } }
    })

    lifecycle.startAgent(agentId).then(
      (res) => {
        setAgents((prev) => {
          const agent = prev[agentId]
          if (!agent) return prev
          return {
            ...prev,
            [agentId]: { ...agent, status: res.status === 'started' ? 'running' : 'idle' },
          }
        })
      },
      (err) => {
        console.error(`Failed to start agent ${agentId}:`, err)
        setAgents((prev) => {
          const agent = prev[agentId]
          if (!agent) return prev
          return { ...prev, [agentId]: { ...agent, status: 'error' } }
        })
      },
    )
  }, [])

  const stopAgent = useCallback((agentId: string) => {
    // Optimistic update
    setAgents((prev) => {
      const agent = prev[agentId]
      if (!agent) return prev
      return { ...prev, [agentId]: { ...agent, status: 'stopping' } }
    })

    lifecycle.stopAgent(agentId).then(
      () => {
        setAgents((prev) => {
          const agent = prev[agentId]
          if (!agent) return prev
          return { ...prev, [agentId]: { ...agent, status: 'idle' } }
        })
      },
      (err) => {
        console.error(`Failed to stop agent ${agentId}:`, err)
        setAgents((prev) => {
          const agent = prev[agentId]
          if (!agent) return prev
          return { ...prev, [agentId]: { ...agent, status: 'error' } }
        })
      },
    )
  }, [])

  const deleteAgent = useCallback((agentId: string) => {
    setAgents((prev) => {
      const next = { ...prev }
      delete next[agentId]
      return next
    })
  }, [])

  const setViewMode = useCallback((agentId: string, mode: AgentViewMode) => {
    setAgents((prev) => {
      const agent = prev[agentId]
      if (!agent) return prev
      return { ...prev, [agentId]: { ...agent, viewMode: mode } }
    })
  }, [])

  return (
    <AgentCanvasContext.Provider value={{ sendMessage, startAgent, stopAgent, deleteAgent, setViewMode, agents }}>
      {children}
    </AgentCanvasContext.Provider>
  )
}
