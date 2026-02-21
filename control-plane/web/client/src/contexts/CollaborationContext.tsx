/**
 * CollaborationContext — React context that manages a Yjs CRDT room.
 *
 * Provides:
 *  - Real-time presence awareness (remote cursors, selections, view modes)
 *  - Shared agent state synced across all peers
 *  - Canvas position sync (node drags propagate to all users)
 *  - Edge sync (connections created by any user are visible to all)
 */

import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  useCallback,
  type ReactNode,
} from 'react'
import * as Y from 'yjs'
import { createCollaborationRoom, type CollaborationRoom } from '@/lib/collaboration'
import type { CollaborationUser, UserPresence } from '@/types/collaboration'

interface CollaborationContextValue {
  /** Whether the WebSocket is connected */
  connected: boolean
  /** Current room reference (null if not joined) */
  room: CollaborationRoom | null
  /** Number of remote peers */
  peerCount: number
  /** Remote users' presence data */
  remoteUsers: Map<number, UserPresence>
  /** Local user info */
  localUser: CollaborationUser | null
  /** Current joined room ID */
  currentRoomId: string | null
  /** Join a collaboration room */
  joinRoom: (roomId: string, user: CollaborationUser) => void
  /** Leave the current room */
  leaveRoom: () => void
  /** Update local cursor position (pass null when cursor leaves canvas) */
  updateCursor: (pos: { x: number; y: number } | null) => void
  /** Update which nodes the local user has selected */
  updateSelection: (nodeIds: string[]) => void
  /** Update the local user's view mode */
  updateViewMode: (mode: 'canvas' | 'list' | 'grid') => void
  /** Update which agent detail panel is open */
  updateActiveAgent: (agentId: string | null) => void
  /** Shared Yjs document (for advanced bindings) */
  ydoc: Y.Doc | null
}

const CollaborationCtx = createContext<CollaborationContextValue | null>(null)

export function useCollaboration(): CollaborationContextValue {
  const ctx = useContext(CollaborationCtx)
  if (!ctx) throw new Error('useCollaboration must be inside <CollaborationProvider>')
  return ctx
}

interface CollaborationProviderProps {
  children: ReactNode
  /** Auto-join a room on mount (optional) */
  roomId?: string
  /** Local user (required if roomId is set) */
  user?: CollaborationUser
  /** WebSocket server URL override */
  wsUrl?: string
}

export function CollaborationProvider({
  children,
  roomId: initialRoomId,
  user: initialUser,
  wsUrl,
}: CollaborationProviderProps) {
  const roomRef = useRef<CollaborationRoom | null>(null)
  const [connected, setConnected] = useState(false)
  const [peerCount, setPeerCount] = useState(0)
  const [remoteUsers, setRemoteUsers] = useState<Map<number, UserPresence>>(new Map())
  const [localUser, setLocalUser] = useState<CollaborationUser | null>(initialUser ?? null)
  const [currentRoomId, setCurrentRoomId] = useState<string | null>(initialRoomId ?? null)

  // Awareness change handler — refreshes remote user list
  const refreshPresence = useCallback(() => {
    const room = roomRef.current
    if (!room) return
    setRemoteUsers(new Map(room.getRemoteUsers()))
    // peer count = remote awareness states (excluding local)
    const states = room.provider.awareness.getStates()
    setPeerCount(Math.max(0, states.size - 1))
  }, [])

  const joinRoom = useCallback(
    (roomId: string, user: CollaborationUser) => {
      // Clean up previous room if any
      if (roomRef.current) {
        roomRef.current.destroy()
        roomRef.current = null
      }

      const room = createCollaborationRoom(roomId, user, wsUrl)
      roomRef.current = room
      setLocalUser(user)
      setCurrentRoomId(roomId)

      // Connection status
      room.provider.on('status', (evt: { status: string }) => {
        setConnected(evt.status === 'connected')
      })

      // Awareness changes → refresh presence
      room.provider.awareness.on('change', refreshPresence)
      // Also refresh on sync
      room.provider.on('sync', refreshPresence)

      // Initial sync
      if (room.provider.wsconnected) {
        setConnected(true)
      }
    },
    [wsUrl, refreshPresence],
  )

  const leaveRoom = useCallback(() => {
    if (roomRef.current) {
      roomRef.current.destroy()
      roomRef.current = null
    }
    setConnected(false)
    setPeerCount(0)
    setRemoteUsers(new Map())
    setCurrentRoomId(null)
  }, [])

  // Auto-join if initialRoomId and initialUser are provided
  useEffect(() => {
    if (initialRoomId && initialUser) {
      joinRoom(initialRoomId, initialUser)
    }
    return () => {
      if (roomRef.current) {
        roomRef.current.destroy()
        roomRef.current = null
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const updateCursor = useCallback((pos: { x: number; y: number } | null) => {
    roomRef.current?.updateCursor(pos)
  }, [])

  const updateSelection = useCallback((nodeIds: string[]) => {
    roomRef.current?.updateSelection(nodeIds)
  }, [])

  const updateViewMode = useCallback((mode: 'canvas' | 'list' | 'grid') => {
    roomRef.current?.updateViewMode(mode)
  }, [])

  const updateActiveAgent = useCallback((agentId: string | null) => {
    roomRef.current?.updateActiveAgent(agentId)
  }, [])

  const value: CollaborationContextValue = {
    connected,
    room: roomRef.current,
    peerCount,
    remoteUsers,
    localUser,
    currentRoomId,
    joinRoom,
    leaveRoom,
    updateCursor,
    updateSelection,
    updateViewMode,
    updateActiveAgent,
    ydoc: roomRef.current?.doc ?? null,
  }

  return (
    <CollaborationCtx.Provider value={value}>
      {children}
    </CollaborationCtx.Provider>
  )
}
