import { useState, useCallback } from 'react'
import { cn } from '@/lib/utils'
import { X, Copy, Check, Users, Eye, Chat, Terminal, Shield } from '@/components/ui/icon-bridge'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import type { SpaceRole, SpaceMember } from '@/types/space'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface ShareDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  spaceId: string
  spaceName: string
  currentRoomId?: string | null
}

// ---------------------------------------------------------------------------
// Role definitions
// ---------------------------------------------------------------------------

interface RoleOption {
  role: SpaceRole
  label: string
  description: string
  icon: typeof Eye
}

const ROLE_OPTIONS: RoleOption[] = [
  {
    role: 'viewer',
    label: 'Viewer',
    description: 'Can watch bots work and read chat',
    icon: Eye,
  },
  {
    role: 'commenter',
    label: 'Commenter',
    description: 'Can watch and comment in chat',
    icon: Chat,
  },
  {
    role: 'operator',
    label: 'Operator',
    description: 'Can control bots and interact',
    icon: Terminal,
  },
  {
    role: 'admin',
    label: 'Admin',
    description: 'Full control including bot management',
    icon: Shield,
  },
]

const ROLE_BADGE_VARIANT: Record<SpaceRole, 'default' | 'secondary' | 'outline' | 'pill'> = {
  viewer: 'secondary',
  commenter: 'default',
  operator: 'pill',
  admin: 'outline',
}

// ---------------------------------------------------------------------------
// Placeholder members (replace with real collaboration context)
// ---------------------------------------------------------------------------

const PLACEHOLDER_MEMBERS: SpaceMember[] = [
  {
    userId: 'self',
    name: 'You',
    color: '#6366f1',
    role: 'admin',
    joinedAt: new Date().toISOString(),
    online: true,
  },
]

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ShareDialog({
  open,
  onOpenChange,
  spaceId,
  spaceName,
  currentRoomId,
}: ShareDialogProps) {
  const [defaultRole, setDefaultRole] = useState<SpaceRole>('viewer')
  const [copied, setCopied] = useState(false)

  // Build the share URL
  const shareUrl = buildShareUrl(spaceId, currentRoomId ?? null)

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(shareUrl)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard API may be unavailable in insecure contexts
    }
  }, [shareUrl])

  // Use placeholder members until a real collaboration context is wired up
  const members: SpaceMember[] = PLACEHOLDER_MEMBERS

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={cn(
          'max-w-md bg-card border-border/60 rounded-xl backdrop-blur-xl',
          'data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95',
        )}
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Users size={18} className="text-muted-foreground" />
            Share {spaceName}
          </DialogTitle>
          <DialogDescription>
            Invite others to collaborate in this space.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-5">
          {/* ---- Section 1: Share Link ---- */}
          <section className="space-y-2">
            <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Share Link
            </h3>
            <div className="flex items-center gap-2">
              <div
                className={cn(
                  'flex-1 truncate rounded-md border border-border/60 bg-muted/30 px-3 py-1.5',
                  'text-xs font-mono text-foreground select-all',
                )}
              >
                {shareUrl}
              </div>
              <Button
                variant="outline"
                size="icon-sm"
                onClick={handleCopy}
                aria-label="Copy share link"
                className="shrink-0"
              >
                {copied ? (
                  <Check size={14} className="text-emerald-400" />
                ) : (
                  <Copy size={14} />
                )}
              </Button>
            </div>
          </section>

          {/* ---- Section 2: Default Access ---- */}
          <section className="space-y-2">
            <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Default Access
            </h3>
            <div className="grid grid-cols-2 gap-2">
              {ROLE_OPTIONS.map(({ role, label, description, icon: Icon }) => {
                const active = defaultRole === role
                return (
                  <button
                    key={role}
                    type="button"
                    onClick={() => setDefaultRole(role)}
                    className={cn(
                      'flex items-start gap-2.5 rounded-lg border p-2.5 text-left',
                      'transition-all duration-150',
                      'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                      active
                        ? 'border-primary/60 bg-primary/10 shadow-sm'
                        : 'border-border/40 bg-card/50 hover:border-border hover:bg-accent/50',
                    )}
                  >
                    <Icon
                      size={16}
                      weight={active ? 'fill' : 'regular'}
                      className={cn(
                        'mt-0.5 shrink-0',
                        active ? 'text-primary' : 'text-muted-foreground',
                      )}
                    />
                    <div className="min-w-0">
                      <div
                        className={cn(
                          'text-xs font-medium',
                          active ? 'text-foreground' : 'text-foreground/80',
                        )}
                      >
                        {label}
                      </div>
                      <div className="text-[10px] leading-tight text-muted-foreground">
                        {description}
                      </div>
                    </div>
                  </button>
                )
              })}
            </div>
          </section>

          {/* ---- Section 3: People with access ---- */}
          <section className="space-y-2">
            <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              People with access
            </h3>
            <div className="space-y-1.5">
              {members.map((member) => (
                <div
                  key={member.userId}
                  className="flex items-center gap-2.5 rounded-md px-2 py-1.5"
                >
                  {/* Avatar circle */}
                  <div
                    className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-[10px] font-bold text-white"
                    style={{ backgroundColor: member.color }}
                  >
                    {member.name.charAt(0).toUpperCase()}
                  </div>

                  {/* Name + online indicator */}
                  <div className="flex min-w-0 flex-1 items-center gap-1.5">
                    <span className="truncate text-xs font-medium text-foreground">
                      {member.name}
                    </span>
                    {member.online && (
                      <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-emerald-400" />
                    )}
                  </div>

                  {/* Role badge */}
                  <Badge variant={ROLE_BADGE_VARIANT[member.role]} size="sm">
                    {member.role}
                  </Badge>
                </div>
              ))}

              {members.length === 0 && (
                <p className="py-3 text-center text-xs text-muted-foreground">
                  No one else has access yet.
                </p>
              )}
            </div>
          </section>
        </div>

        {/* ---- Footer ---- */}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Done
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function buildShareUrl(spaceId: string, roomId: string | null): string {
  const origin =
    typeof window !== 'undefined' ? window.location.origin : 'https://app.hanzo.ai'
  const params = new URLSearchParams({ space: spaceId })
  if (roomId) {
    params.set('room', roomId)
  }
  return `${origin}/ui/playground?${params.toString()}`
}
