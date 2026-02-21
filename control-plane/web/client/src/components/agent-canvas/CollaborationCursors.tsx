/**
 * CollaborationCursors — renders remote users' cursors on the canvas.
 *
 * Each cursor is a colored arrow with the user's name floating beside it.
 * Cursors are absolutely positioned over the ReactFlow viewport.
 */

import { useCollaboration } from '@/contexts/CollaborationContext'
import { cn } from '@/lib/utils'

export function CollaborationCursors() {
  const { remoteUsers, connected } = useCollaboration()

  if (!connected) return null

  const cursors = Array.from(remoteUsers.entries())
    .filter(([, presence]) => presence.cursor !== null)
    .map(([clientId, presence]) => ({
      clientId,
      ...presence,
    }))

  if (cursors.length === 0) return null

  return (
    <div className="pointer-events-none absolute inset-0 z-50 overflow-hidden">
      {cursors.map((c) => (
        <div
          key={c.clientId}
          className="absolute transition-all duration-75 ease-out"
          style={{
            left: c.cursor!.x,
            top: c.cursor!.y,
            transform: 'translate(-2px, -2px)',
          }}
        >
          {/* Cursor arrow SVG */}
          <svg
            width="16"
            height="20"
            viewBox="0 0 16 20"
            fill="none"
            className="drop-shadow-sm"
          >
            <path
              d="M0.5 0.5L15.5 10L8 11.5L5 19.5L0.5 0.5Z"
              fill={c.user.color}
              stroke="white"
              strokeWidth="1"
            />
          </svg>

          {/* Name label */}
          <div
            className={cn(
              'absolute left-4 top-3 whitespace-nowrap rounded-md px-1.5 py-0.5',
              'text-[10px] font-medium text-white shadow-sm',
            )}
            style={{ backgroundColor: c.user.color }}
          >
            {c.user.name}
            {c.user.isBot && (
              <span className="ml-1 opacity-80">bot</span>
            )}
          </div>
        </div>
      ))}
    </div>
  )
}
