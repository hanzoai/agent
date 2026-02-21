import { useEffect, useRef, useState } from "react"
import { cn } from "@/lib/utils"
import { Plus, X } from "@/components/ui/icon-bridge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { useSpaces } from "@/contexts/SpaceContext"
import { SpaceCreateDialog } from "./SpaceCreateDialog"

export function SpaceTabs() {
  const { spaces, activeSpaceId, setActiveSpace, createSpace, deleteSpace, renameSpace } =
    useSpaces()

  const [showCreate, setShowCreate] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editValue, setEditValue] = useState("")
  const editInputRef = useRef<HTMLInputElement>(null)
  const createBtnRef = useRef<HTMLButtonElement>(null)
  const scrollRef = useRef<HTMLDivElement>(null)

  // Focus inline rename input when editing starts
  useEffect(() => {
    if (editingId && editInputRef.current) {
      editInputRef.current.focus()
      editInputRef.current.select()
    }
  }, [editingId])

  const startRename = (id: string, currentName: string) => {
    setEditingId(id)
    setEditValue(currentName)
  }

  const commitRename = () => {
    if (editingId && editValue.trim()) {
      renameSpace(editingId, editValue.trim())
    }
    setEditingId(null)
    setEditValue("")
  }

  const handleRenameKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault()
      commitRename()
    }
    if (e.key === "Escape") {
      e.preventDefault()
      setEditingId(null)
      setEditValue("")
    }
  }

  const handleCreate = (name: string, emoji: string) => {
    createSpace(name, emoji)
    setShowCreate(false)
  }

  return (
    <div className="relative flex items-center bg-card/80 border-b border-border/50 px-2 py-1">
      {/* Scrollable tab container */}
      <div
        ref={scrollRef}
        className="flex items-center gap-1 overflow-x-auto scrollbar-none"
      >
        {spaces.map((space) => {
          const isActive = space.id === activeSpaceId
          const isEditing = space.id === editingId

          return (
            <div
              key={space.id}
              className={cn(
                "group relative flex shrink-0 items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors select-none",
                isActive
                  ? "bg-primary/10 text-primary"
                  : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
              )}
              role="tab"
              tabIndex={0}
              aria-selected={isActive}
              onClick={() => {
                if (!isEditing) setActiveSpace(space.id)
              }}
              onDoubleClick={() => startRename(space.id, space.name)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !isEditing) setActiveSpace(space.id)
              }}
            >
              {/* Emoji */}
              <span className="text-sm leading-none">{space.emoji}</span>

              {/* Name or inline edit */}
              {isEditing ? (
                <Input
                  ref={editInputRef}
                  value={editValue}
                  onChange={(e) => setEditValue(e.target.value)}
                  onBlur={commitRename}
                  onKeyDown={handleRenameKeyDown}
                  className="h-5 w-24 border-none bg-transparent px-0.5 py-0 text-xs shadow-none focus-visible:ring-1"
                />
              ) : (
                <span className="max-w-[120px] truncate">{space.name}</span>
              )}

              {/* Delete button -- hidden for last space */}
              {spaces.length > 1 && !isEditing && (
                <button
                  type="button"
                  className={cn(
                    "ml-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-sm transition-opacity",
                    "opacity-0 group-hover:opacity-70 hover:!opacity-100 hover:bg-destructive/15 hover:text-destructive",
                  )}
                  onClick={(e) => {
                    e.stopPropagation()
                    deleteSpace(space.id)
                  }}
                  aria-label={`Delete space ${space.name}`}
                >
                  <X size={10} />
                </button>
              )}
            </div>
          )
        })}
      </div>

      {/* Add space button */}
      <div className="relative ml-1 shrink-0">
        <Button
          ref={createBtnRef}
          variant="ghost"
          size="icon-sm"
          className="h-6 w-6"
          onClick={() => setShowCreate((prev) => !prev)}
          aria-label="Create space"
        >
          <Plus size={14} />
        </Button>

        {showCreate && (
          <SpaceCreateDialog
            onSubmit={handleCreate}
            onCancel={() => setShowCreate(false)}
          />
        )}
      </div>
    </div>
  )
}
