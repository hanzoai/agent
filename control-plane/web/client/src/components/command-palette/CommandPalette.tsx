import { useEffect, useCallback, useMemo } from 'react'
import type { CommandItem } from '@/types/space'
import { cn } from '@/lib/utils'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem as CommandOption,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import { Dialog, DialogContent } from '@/components/ui/dialog'

// ---------------------------------------------------------------------------
// Category metadata
// ---------------------------------------------------------------------------

const CATEGORY_LABELS: Record<CommandItem['category'], string> = {
  space: 'Spaces',
  bot: 'Bots',
  page: 'Pages',
  action: 'Actions',
}

const CATEGORY_ORDER: CommandItem['category'][] = ['space', 'bot', 'page', 'action']

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface CommandPaletteProps {
  items: CommandItem[]
  open: boolean
  onOpenChange: (open: boolean) => void
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function CommandPalette({ items, open, onOpenChange }: CommandPaletteProps) {
  // ---- Global keyboard shortcut (Cmd+K / Ctrl+K) ----
  const handleGlobalKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'k' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        onOpenChange(!open)
      }
    },
    [open, onOpenChange],
  )

  useEffect(() => {
    document.addEventListener('keydown', handleGlobalKeyDown)
    return () => document.removeEventListener('keydown', handleGlobalKeyDown)
  }, [handleGlobalKeyDown])

  // ---- Group items by category ----
  const grouped = useMemo(() => {
    const map = new Map<CommandItem['category'], CommandItem[]>()
    for (const cat of CATEGORY_ORDER) {
      map.set(cat, [])
    }
    for (const item of items) {
      const list = map.get(item.category)
      if (list) {
        list.push(item)
      }
    }
    return map
  }, [items])

  // ---- Select handler ----
  const handleSelect = useCallback(
    (id: string) => {
      const item = items.find((i) => i.id === id)
      if (item) {
        item.action()
        onOpenChange(false)
      }
    },
    [items, onOpenChange],
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={cn(
          'overflow-hidden p-0 shadow-2xl',
          'bg-card/95 backdrop-blur-xl',
          'border border-border/60',
          'rounded-xl',
          'max-w-lg',
          // Remove default close button padding that DialogContent adds
          '[&>button:last-child]:hidden',
        )}
      >
        <Command
          className={cn(
            'bg-transparent',
            '[&_[cmdk-group-heading]]:px-3 [&_[cmdk-group-heading]]:py-2',
            '[&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-medium',
            '[&_[cmdk-group-heading]]:text-muted-foreground [&_[cmdk-group-heading]]:uppercase',
            '[&_[cmdk-group-heading]]:tracking-wider',
          )}
          filter={(value, search, keywords) => {
            const haystack = [value, ...(keywords ?? [])].join(' ').toLowerCase()
            const needle = search.toLowerCase()
            // Simple fuzzy: every character of the query appears in order
            let hi = 0
            for (let si = 0; si < needle.length; si++) {
              const idx = haystack.indexOf(needle[si]!, hi)
              if (idx === -1) return 0
              hi = idx + 1
            }
            return 1
          }}
        >
          <CommandInput
            placeholder="Search pages, bots, actions..."
            className="h-12 border-none bg-transparent text-foreground placeholder:text-muted-foreground focus:ring-0"
          />

          <CommandList className="max-h-[360px] overflow-y-auto overflow-x-hidden">
            <CommandEmpty className="py-8 text-center text-sm text-muted-foreground">
              No results found.
            </CommandEmpty>

            {CATEGORY_ORDER.map((cat, catIdx) => {
              const group = grouped.get(cat)
              if (!group || group.length === 0) return null
              return (
                <div key={cat}>
                  {catIdx > 0 && <CommandSeparator className="bg-border/40" />}
                  <CommandGroup heading={CATEGORY_LABELS[cat]}>
                    {group.map((item) => (
                      <CommandOption
                        key={item.id}
                        value={item.id}
                        keywords={[item.label, ...(item.keywords ?? [])]}
                        onSelect={handleSelect}
                        className={cn(
                          'flex items-center gap-3 rounded-lg px-3 py-2.5 cursor-pointer',
                          'text-foreground',
                          'data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground',
                          'transition-colors',
                        )}
                      >
                        {item.emoji && (
                          <span className="flex h-6 w-6 shrink-0 items-center justify-center text-base">
                            {item.emoji}
                          </span>
                        )}
                        <div className="flex min-w-0 flex-col">
                          <span className="truncate text-sm font-medium">{item.label}</span>
                          {item.description && (
                            <span className="truncate text-xs text-muted-foreground">
                              {item.description}
                            </span>
                          )}
                        </div>
                      </CommandOption>
                    ))}
                  </CommandGroup>
                </div>
              )
            })}
          </CommandList>

          {/* Footer hint */}
          <div className="flex items-center justify-between border-t border-border/40 px-3 py-2 text-xs text-muted-foreground">
            <div className="flex gap-2">
              <kbd className="rounded border border-border/60 bg-background px-1.5 py-0.5 font-mono text-[10px]">
                &uarr;&darr;
              </kbd>
              <span>navigate</span>
            </div>
            <div className="flex gap-2">
              <kbd className="rounded border border-border/60 bg-background px-1.5 py-0.5 font-mono text-[10px]">
                &crarr;
              </kbd>
              <span>select</span>
            </div>
            <div className="flex gap-2">
              <kbd className="rounded border border-border/60 bg-background px-1.5 py-0.5 font-mono text-[10px]">
                esc
              </kbd>
              <span>close</span>
            </div>
          </div>
        </Command>
      </DialogContent>
    </Dialog>
  )
}
