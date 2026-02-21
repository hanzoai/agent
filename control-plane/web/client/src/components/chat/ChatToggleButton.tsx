"use client"

import * as React from "react"
import { cn } from "@/lib/utils"
import { Chat } from "@/components/ui/icon-bridge"
import { Button } from "@/components/ui/button"

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface ChatToggleButtonProps {
  open: boolean
  onToggle: () => void
  unreadCount?: number
  className?: string
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ChatToggleButton({
  open,
  onToggle,
  unreadCount = 0,
  className,
}: ChatToggleButtonProps) {
  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={onToggle}
      aria-label={open ? "Close chat" : "Open chat"}
      className={cn("relative", className)}
    >
      <Chat size={18} weight={open ? "fill" : "regular"} />

      {/* Unread count badge */}
      {unreadCount > 0 && (
        <span
          className={cn(
            "absolute -top-0.5 -right-0.5 flex items-center justify-center",
            "min-w-[16px] h-4 rounded-full px-1",
            "bg-destructive text-destructive-foreground",
            "text-[10px] font-semibold leading-none",
            // Pulse animation for new messages
            "animate-in zoom-in-50 duration-200",
          )}
        >
          {unreadCount > 99 ? "99+" : unreadCount}
        </span>
      )}

      {/* Subtle pulse ring when there are unread messages */}
      {unreadCount > 0 && (
        <span
          className="absolute inset-0 rounded-md animate-pulse ring-1 ring-destructive/40 pointer-events-none"
          aria-hidden
        />
      )}
    </Button>
  )
}
