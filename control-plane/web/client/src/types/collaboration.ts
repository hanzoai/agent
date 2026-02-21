// Collaboration types for real-time CRDT-based multi-user canvas

export interface CollaborationUser {
  id: string
  name: string
  color: string
  avatar?: string
  isBot?: boolean
}

export interface UserPresence {
  user: CollaborationUser
  cursor: { x: number; y: number } | null
  selectedNodes: string[]
  viewMode: 'canvas' | 'list' | 'grid'
  activeAgent: string | null
  lastSeen: number
}

export interface CollaborationState {
  /** Whether the collaboration provider is connected */
  connected: boolean
  /** Current room/project ID */
  roomId: string | null
  /** Local user info */
  localUser: CollaborationUser | null
  /** All remote users' presence */
  remoteUsers: Map<number, UserPresence>
  /** Number of connected peers */
  peerCount: number
}

export type CollaborationEvent =
  | { type: 'connected' }
  | { type: 'disconnected' }
  | { type: 'peer-joined'; user: CollaborationUser }
  | { type: 'peer-left'; user: CollaborationUser }
  | { type: 'sync-complete' }
