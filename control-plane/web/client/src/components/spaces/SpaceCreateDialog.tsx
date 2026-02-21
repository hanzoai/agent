import { useRef, useState } from "react"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

const PRESET_EMOJIS = ["\u{1F680}", "\u{1F4BB}", "\u{1F3A8}", "\u{1F4CA}", "\u{1F52C}", "\u{1F3AF}", "\u26A1", "\u{1F31F}"]

interface SpaceCreateDialogProps {
  onSubmit: (name: string, emoji: string) => void
  onCancel: () => void
  className?: string
}

export function SpaceCreateDialog({ onSubmit, onCancel, className }: SpaceCreateDialogProps) {
  const [name, setName] = useState("")
  const [emoji, setEmoji] = useState(PRESET_EMOJIS[0])
  const inputRef = useRef<HTMLInputElement>(null)

  const handleSubmit = () => {
    const trimmed = name.trim()
    if (!trimmed) return
    onSubmit(trimmed, emoji)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault()
      handleSubmit()
    }
    if (e.key === "Escape") {
      e.preventDefault()
      onCancel()
    }
  }

  return (
    <div
      className={cn(
        "absolute top-full left-0 z-50 mt-1 w-64 rounded-lg border border-border bg-popover p-3 shadow-lg",
        className,
      )}
      onKeyDown={handleKeyDown}
    >
      {/* Emoji picker row */}
      <div className="mb-2 flex gap-1">
        {PRESET_EMOJIS.map((e) => (
          <button
            key={e}
            type="button"
            className={cn(
              "flex h-7 w-7 items-center justify-center rounded text-sm transition-colors",
              emoji === e
                ? "bg-primary/15 ring-1 ring-primary/40"
                : "hover:bg-accent",
            )}
            onClick={() => setEmoji(e)}
          >
            {e}
          </button>
        ))}
      </div>

      {/* Name input */}
      <div className="mb-2">
        <Input
          ref={inputRef}
          autoFocus
          placeholder="Space name..."
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="h-7 text-xs"
        />
      </div>

      {/* Actions */}
      <div className="flex justify-end gap-1.5">
        <Button variant="ghost" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button size="sm" onClick={handleSubmit} disabled={!name.trim()}>
          Create
        </Button>
      </div>
    </div>
  )
}
