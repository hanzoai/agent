"use client"

import * as React from "react"
import { useCallback, useEffect, useRef, useState } from "react"
import { cn } from "@/lib/utils"
import { X, Chat, Users } from "@/components/ui/icon-bridge"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { ArrowUpIcon } from "@phosphor-icons/react/dist/csr/ArrowUp"
import type { ChatMessage } from "@/types/space"

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface SideChatPanelProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  spaceId: string
  className?: string
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Return a stable colour for a given userId so avatars are deterministic. */
const PALETTE = [
  "#6366f1", "#f43f5e", "#10b981", "#f59e0b",
  "#3b82f6", "#8b5cf6", "#ec4899", "#14b8a6",
]

function colorForUser(userId: string): string {
  let hash = 0
  for (let i = 0; i < userId.length; i++) {
    hash = ((hash << 5) - hash + userId.charCodeAt(i)) | 0
  }
  return PALETTE[Math.abs(hash) % PALETTE.length]
}

function makeId(): string {
  return Math.random().toString(36).slice(2, 10)
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso)
    return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })
  } catch {
    return ""
  }
}

/** Render message content, highlighting @mentions in blue. */
function renderContent(content: string): React.ReactNode {
  const parts = content.split(/(@\w+)/g)
  return parts.map((part, i) =>
    part.startsWith("@") ? (
      <span key={i} className="text-blue-400 font-medium">
        {part}
      </span>
    ) : (
      <span key={i}>{part}</span>
    )
  )
}

// ---------------------------------------------------------------------------
// Stub user -- will be replaced with real auth context later
// ---------------------------------------------------------------------------

const CURRENT_USER = {
  id: "self",
  name: "You",
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function SideChatPanel({
  open,
  onOpenChange,
  spaceId,
  className,
}: SideChatPanelProps) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [draft, setDraft] = useState("")
  const bottomRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  // Auto-scroll to bottom when messages change
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" })
  }, [messages])

  // Focus input when panel opens
  useEffect(() => {
    if (open) {
      // Small delay so the slide animation finishes first
      const t = setTimeout(() => inputRef.current?.focus(), 200)
      return () => clearTimeout(t)
    }
  }, [open])

  const sendMessage = useCallback(() => {
    const text = draft.trim()
    if (!text) return

    const msg: ChatMessage = {
      id: makeId(),
      spaceId,
      userId: CURRENT_USER.id,
      userName: CURRENT_USER.name,
      userColor: colorForUser(CURRENT_USER.id),
      content: text,
      timestamp: new Date().toISOString(),
    }

    setMessages((prev) => [...prev, msg])
    setDraft("")
  }, [draft, spaceId])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault()
        sendMessage()
      }
    },
    [sendMessage]
  )

  // Member count is derived from unique userIds in messages (minimum 1 = self)
  const memberCount = Math.max(
    1,
    new Set(messages.map((m) => m.userId)).size
  )

  return (
    <div
      className={cn(
        // Layout
        "fixed top-0 right-0 z-40 h-full w-[320px]",
        "flex flex-col",
        // Appearance
        "bg-card border-l border-border/60",
        // Slide animation
        "transition-transform duration-200 ease-in-out",
        open ? "translate-x-0" : "translate-x-full",
        className
      )}
    >
      {/* ------- Header ------- */}
      <div className="flex items-center justify-between gap-2 border-b border-border/60 px-3 py-2">
        <div className="flex items-center gap-2">
          <Chat size={16} className="text-muted-foreground" />
          <span className="text-sm font-medium text-foreground">Chat</span>
          <Badge variant="secondary" size="sm" showIcon={false}>
            <Users size={10} className="mr-0.5" />
            {memberCount}
          </Badge>
        </div>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => onOpenChange(false)}
          aria-label="Close chat"
        >
          <X size={14} />
        </Button>
      </div>

      {/* ------- Message list ------- */}
      <div className="flex-1 overflow-y-auto px-3 py-2 space-y-3">
        {messages.length === 0 && (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground text-xs select-none gap-2">
            <Chat size={24} className="opacity-40" />
            <span>No messages yet</span>
          </div>
        )}

        {messages.map((msg) => (
          <div key={msg.id} className="flex gap-2 group">
            {/* Avatar */}
            <div
              className="flex-shrink-0 w-6 h-6 rounded-full flex items-center justify-center text-[10px] font-bold text-white select-none mt-0.5"
              style={{ backgroundColor: msg.userColor }}
              title={msg.userName}
            >
              {msg.userName.charAt(0).toUpperCase()}
            </div>

            {/* Content */}
            <div className="flex-1 min-w-0">
              <div className="flex items-baseline gap-1.5">
                <span className="text-xs font-medium text-foreground truncate">
                  {msg.userName}
                </span>
                <span className="text-[10px] text-muted-foreground">
                  {formatTime(msg.timestamp)}
                </span>
              </div>
              <p className="text-xs text-foreground/90 whitespace-pre-wrap break-words leading-relaxed mt-0.5">
                {renderContent(msg.content)}
              </p>
            </div>
          </div>
        ))}

        <div ref={bottomRef} />
      </div>

      {/* ------- Input area ------- */}
      <div className="border-t border-border/60 px-3 py-2">
        <div className="flex items-end gap-1.5">
          <textarea
            ref={inputRef}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type a message..."
            rows={1}
            className={cn(
              "flex-1 resize-none rounded-md border border-border/60 bg-background px-2.5 py-1.5",
              "text-xs text-foreground placeholder:text-muted-foreground",
              "focus:outline-none focus:ring-1 focus:ring-ring",
              "max-h-24 overflow-y-auto"
            )}
          />
          <Button
            variant="default"
            size="icon-sm"
            onClick={sendMessage}
            disabled={!draft.trim()}
            aria-label="Send message"
          >
            <ArrowUpIcon size={14} />
          </Button>
        </div>
        <p className="text-[10px] text-muted-foreground mt-1 select-none">
          Enter to send, Shift+Enter for newline
        </p>
      </div>
    </div>
  )
}
