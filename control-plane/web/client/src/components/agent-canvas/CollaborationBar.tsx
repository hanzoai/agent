/**
 * CollaborationBar — top-level bar showing collaboration status
 * and a "Share" button to enable real-time sync for the canvas.
 */

import { useState, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ShareNetwork } from '@/components/ui/icon-bridge'
import { useCollaboration } from '@/contexts/CollaborationContext'
import { PresenceAvatars } from './PresenceAvatars'
import { pickColor } from '@/lib/collaboration'
import { cn } from '@/lib/utils'
import type { CollaborationUser } from '@/types/collaboration'

interface CollaborationBarProps {
  className?: string
}

export function CollaborationBar({ className }: CollaborationBarProps) {
  const { connected, joinRoom, leaveRoom, peerCount, currentRoomId } = useCollaboration()
  const [showJoinDialog, setShowJoinDialog] = useState(false)
  const [roomInput, setRoomInput] = useState('')
  const [nameInput, setNameInput] = useState('')

  const handleJoin = useCallback(() => {
    if (!roomInput.trim() || !nameInput.trim()) return

    const user: CollaborationUser = {
      id: crypto.randomUUID(),
      name: nameInput.trim(),
      color: pickColor(Math.floor(Math.random() * 16)),
    }

    joinRoom(roomInput.trim(), user)
    setShowJoinDialog(false)
  }, [roomInput, nameInput, joinRoom])

  const handleCopyInvite = useCallback(async () => {
    if (!currentRoomId) return
    const url = new URL(window.location.href)
    url.searchParams.set('room', currentRoomId)
    await navigator.clipboard.writeText(url.toString())
  }, [currentRoomId])

  if (connected) {
    return (
      <div className={cn('flex items-center gap-2', className)}>
        <PresenceAvatars />
        <Button
          variant="outline"
          size="sm"
          onClick={handleCopyInvite}
          className="text-xs"
        >
          Copy Invite
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={leaveRoom}
          className="text-xs text-muted-foreground hover:text-destructive"
        >
          Leave
        </Button>
      </div>
    )
  }

  return (
    <div className={cn('relative flex items-center', className)}>
      <Button
        variant="outline"
        size="sm"
        onClick={() => setShowJoinDialog(!showJoinDialog)}
        className="gap-1.5 text-xs"
      >
        <ShareNetwork size={14} />
        Collaborate
      </Button>

      {showJoinDialog && (
        <>
          <div
            className="fixed inset-0 z-40"
            onClick={() => setShowJoinDialog(false)}
          />
          <div
            className={cn(
              'absolute right-0 top-full mt-1 z-50 w-72',
              'rounded-lg border border-border/60 bg-card p-3 shadow-lg space-y-2',
              'animate-in fade-in-0 zoom-in-95 duration-150',
            )}
          >
            <div className="text-xs font-medium text-foreground">
              Join a collaboration room
            </div>
            <Input
              placeholder="Room ID (e.g. my-project)"
              value={roomInput}
              onChange={(e) => setRoomInput(e.target.value)}
              className="h-7 text-xs"
              autoFocus
            />
            <Input
              placeholder="Your name"
              value={nameInput}
              onChange={(e) => setNameInput(e.target.value)}
              className="h-7 text-xs"
              onKeyDown={(e) => e.key === 'Enter' && handleJoin()}
            />
            <Button
              size="sm"
              className="w-full text-xs"
              onClick={handleJoin}
              disabled={!roomInput.trim() || !nameInput.trim()}
            >
              Join Room
            </Button>
          </div>
        </>
      )}
    </div>
  )
}
