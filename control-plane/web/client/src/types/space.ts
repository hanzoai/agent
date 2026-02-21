/** A Space is a project workspace containing bots, conversations, and shared state. */
export interface Space {
  id: string
  name: string
  emoji: string
  description?: string
  createdAt: string
  updatedAt: string
  /** Collaboration room ID (for Yjs sync) */
  roomId?: string
  /** Share settings */
  sharing?: SpaceSharing
}

export interface SpaceSharing {
  enabled: boolean
  /** Public share link token */
  token?: string
  /** Default role for link joiners */
  defaultRole: SpaceRole
}

export type SpaceRole = 'viewer' | 'commenter' | 'operator' | 'admin'

export interface SpaceMember {
  userId: string
  name: string
  color: string
  role: SpaceRole
  joinedAt: string
  online: boolean
}

export interface ChatMessage {
  id: string
  spaceId: string
  userId: string
  userName: string
  userColor: string
  content: string
  timestamp: string
  /** Reply to another message */
  replyTo?: string
  /** Attached to a specific bot node */
  attachedTo?: string
}

export interface CommandItem {
  id: string
  label: string
  description?: string
  icon?: string
  emoji?: string
  category: 'space' | 'bot' | 'page' | 'action'
  keywords?: string[]
  action: () => void
}
