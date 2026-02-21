/**
 * Core CRDT collaboration module using Yjs.
 *
 * Shared document structure:
 *   YDoc "room:{roomId}"
 *   ├── agents: Y.Map<string, AgentNodeData>   — agent instances
 *   ├── positions: Y.Map<string, {x,y}>        — node positions on canvas
 *   ├── edges: Y.Array<{id,source,target,...}>  — connections between agents
 *   └── meta: Y.Map<string, any>               — room metadata (name, owner, etc.)
 *
 * Awareness layer carries per-user presence: cursor, selected nodes, view mode.
 */

import * as Y from 'yjs'
import { WebsocketProvider } from 'y-websocket'
import type { CollaborationUser, UserPresence } from '@/types/collaboration'

// Default collaboration WebSocket server URL
const DEFAULT_WS_URL = 'ws://localhost:1234'

// Distinct colors for up to 16 users; wraps around after that
const USER_COLORS = [
  '#6366f1', '#f43f5e', '#10b981', '#f59e0b',
  '#8b5cf6', '#ec4899', '#14b8a6', '#ef4444',
  '#3b82f6', '#84cc16', '#d946ef', '#06b6d4',
  '#f97316', '#22d3ee', '#a78bfa', '#fb923c',
]

export function pickColor(index: number): string {
  return USER_COLORS[index % USER_COLORS.length]
}

export interface CollaborationRoom {
  doc: Y.Doc
  provider: WebsocketProvider

  // Shared types
  agents: Y.Map<Record<string, unknown>>
  positions: Y.Map<{ x: number; y: number }>
  edges: Y.Array<Record<string, unknown>>
  meta: Y.Map<unknown>

  // Helpers
  setLocalUser: (user: CollaborationUser) => void
  updateCursor: (cursor: { x: number; y: number } | null) => void
  updateSelection: (nodeIds: string[]) => void
  updateViewMode: (mode: 'canvas' | 'list' | 'grid') => void
  updateActiveAgent: (agentId: string | null) => void
  getRemoteUsers: () => Map<number, UserPresence>
  destroy: () => void
}

/**
 * Create (or join) a collaboration room.
 *
 * @param roomId   - Unique room identifier (e.g. project ID or org slug)
 * @param user     - Local user info
 * @param wsUrl    - WebSocket server URL (defaults to VITE_COLLAB_WS_URL or localhost:1234)
 */
export function createCollaborationRoom(
  roomId: string,
  user: CollaborationUser,
  wsUrl?: string,
): CollaborationRoom {
  const resolvedWsUrl =
    wsUrl ??
    (typeof import.meta !== 'undefined'
      ? (import.meta as Record<string, Record<string, string>>).env?.VITE_COLLAB_WS_URL
      : undefined) ??
    DEFAULT_WS_URL

  const doc = new Y.Doc()

  // Connect WebSocket provider
  const provider = new WebsocketProvider(resolvedWsUrl, `room:${roomId}`, doc, {
    connect: true,
    // Reconnect aggressively
    maxBackoffTime: 5000,
  })

  // Set local awareness state
  provider.awareness.setLocalStateField('user', user)
  provider.awareness.setLocalStateField('cursor', null)
  provider.awareness.setLocalStateField('selectedNodes', [])
  provider.awareness.setLocalStateField('viewMode', 'canvas')
  provider.awareness.setLocalStateField('activeAgent', null)
  provider.awareness.setLocalStateField('lastSeen', Date.now())

  // Shared CRDT types
  const agents = doc.getMap<Record<string, unknown>>('agents')
  const positions = doc.getMap<{ x: number; y: number }>('positions')
  const edges = doc.getArray<Record<string, unknown>>('edges')
  const meta = doc.getMap<unknown>('meta')

  // Helpers to update local awareness
  const setLocalUser = (u: CollaborationUser) => {
    provider.awareness.setLocalStateField('user', u)
  }

  const updateCursor = (cursor: { x: number; y: number } | null) => {
    provider.awareness.setLocalStateField('cursor', cursor)
    provider.awareness.setLocalStateField('lastSeen', Date.now())
  }

  const updateSelection = (nodeIds: string[]) => {
    provider.awareness.setLocalStateField('selectedNodes', nodeIds)
  }

  const updateViewMode = (mode: 'canvas' | 'list' | 'grid') => {
    provider.awareness.setLocalStateField('viewMode', mode)
  }

  const updateActiveAgent = (agentId: string | null) => {
    provider.awareness.setLocalStateField('activeAgent', agentId)
  }

  const getRemoteUsers = (): Map<number, UserPresence> => {
    const states = provider.awareness.getStates()
    const result = new Map<number, UserPresence>()
    const localClientId = provider.awareness.clientID

    states.forEach((state, clientId) => {
      if (clientId === localClientId) return
      if (!state.user) return
      result.set(clientId, {
        user: state.user as CollaborationUser,
        cursor: (state.cursor as { x: number; y: number } | null) ?? null,
        selectedNodes: (state.selectedNodes as string[]) ?? [],
        viewMode: (state.viewMode as 'canvas' | 'list' | 'grid') ?? 'canvas',
        activeAgent: (state.activeAgent as string | null) ?? null,
        lastSeen: (state.lastSeen as number) ?? Date.now(),
      })
    })

    return result
  }

  const destroy = () => {
    provider.disconnect()
    provider.destroy()
    doc.destroy()
  }

  return {
    doc,
    provider,
    agents,
    positions,
    edges,
    meta,
    setLocalUser,
    updateCursor,
    updateSelection,
    updateViewMode,
    updateActiveAgent,
    getRemoteUsers,
    destroy,
  }
}
