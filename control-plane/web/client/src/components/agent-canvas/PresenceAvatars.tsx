/**
 * PresenceAvatars — shows who's online in the collaboration room.
 *
 * Displayed in the canvas toolbar: stacked avatar circles + a count badge.
 * Clicking reveals a dropdown with user names, view modes, and active agents.
 */

import { useState } from 'react'
import { useCollaboration } from '@/contexts/CollaborationContext'
import { cn } from '@/lib/utils'

export function PresenceAvatars() {
  const { connected, peerCount, remoteUsers, localUser } = useCollaboration()
  const [expanded, setExpanded] = useState(false)

  if (!connected) {
    return (
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground/60">
        <span className="h-2 w-2 rounded-full bg-muted-foreground/30" />
        Offline
      </div>
    )
  }

  const users = Array.from(remoteUsers.values())
  const totalOnline = peerCount + 1 // +1 for local user

  return (
    <div className="relative">
      <button
        type="button"
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-1.5 rounded-md px-2 py-1 hover:bg-muted/30 transition-colors"
      >
        {/* Stacked avatars */}
        <div className="flex -space-x-1.5">
          {/* Local user */}
          {localUser && (
            <div
              className="h-5 w-5 rounded-full border-2 border-card flex items-center justify-center text-[8px] font-bold text-white"
              style={{ backgroundColor: localUser.color }}
              title={`${localUser.name} (you)`}
            >
              {localUser.name.charAt(0).toUpperCase()}
            </div>
          )}

          {/* Remote users (show up to 4) */}
          {users.slice(0, 4).map((presence, i) => (
            <div
              key={i}
              className="h-5 w-5 rounded-full border-2 border-card flex items-center justify-center text-[8px] font-bold text-white"
              style={{ backgroundColor: presence.user.color }}
              title={presence.user.name}
            >
              {presence.user.isBot ? '~' : presence.user.name.charAt(0).toUpperCase()}
            </div>
          ))}

          {/* Overflow */}
          {users.length > 4 && (
            <div className="h-5 w-5 rounded-full border-2 border-card bg-muted flex items-center justify-center text-[8px] font-medium text-muted-foreground">
              +{users.length - 4}
            </div>
          )}
        </div>

        {/* Count */}
        <span className="text-xs text-muted-foreground">
          {totalOnline} online
        </span>

        {/* Live dot */}
        <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 animate-pulse" />
      </button>

      {/* Expanded dropdown */}
      {expanded && (
        <div
          className={cn(
            'absolute right-0 top-full mt-1 z-50 w-64',
            'rounded-lg border border-border/60 bg-card shadow-lg',
            'animate-in fade-in-0 zoom-in-95 duration-150',
          )}
        >
          <div className="border-b border-border/40 px-3 py-2">
            <div className="text-xs font-medium text-foreground">
              {totalOnline} {totalOnline === 1 ? 'user' : 'users'} online
            </div>
          </div>

          <div className="max-h-48 overflow-y-auto py-1">
            {/* Local user */}
            {localUser && (
              <div className="flex items-center gap-2 px-3 py-1.5">
                <div
                  className="h-5 w-5 rounded-full flex items-center justify-center text-[8px] font-bold text-white"
                  style={{ backgroundColor: localUser.color }}
                >
                  {localUser.name.charAt(0).toUpperCase()}
                </div>
                <div className="flex-1 min-w-0">
                  <div className="text-xs font-medium text-foreground truncate">
                    {localUser.name} <span className="text-muted-foreground">(you)</span>
                  </div>
                </div>
              </div>
            )}

            {/* Remote users */}
            {users.map((presence, i) => (
              <div key={i} className="flex items-center gap-2 px-3 py-1.5">
                <div
                  className="h-5 w-5 rounded-full flex items-center justify-center text-[8px] font-bold text-white"
                  style={{ backgroundColor: presence.user.color }}
                >
                  {presence.user.isBot ? '~' : presence.user.name.charAt(0).toUpperCase()}
                </div>
                <div className="flex-1 min-w-0">
                  <div className="text-xs font-medium text-foreground truncate">
                    {presence.user.name}
                    {presence.user.isBot && (
                      <span className="ml-1 text-[10px] text-muted-foreground">bot</span>
                    )}
                  </div>
                  <div className="text-[10px] text-muted-foreground">
                    {presence.activeAgent
                      ? `Viewing ${presence.activeAgent}`
                      : presence.viewMode}
                  </div>
                </div>
                {/* Selection indicator */}
                {presence.selectedNodes.length > 0 && (
                  <span className="text-[10px] text-muted-foreground">
                    {presence.selectedNodes.length} selected
                  </span>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Click-away overlay */}
      {expanded && (
        <div
          className="fixed inset-0 z-40"
          onClick={() => setExpanded(false)}
        />
      )}
    </div>
  )
}
