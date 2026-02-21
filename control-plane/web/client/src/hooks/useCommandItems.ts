import { useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTheme } from 'next-themes'
import type { CommandItem } from '@/types/space'

// ---------------------------------------------------------------------------
// Optional dynamic data
// ---------------------------------------------------------------------------

interface UseCommandItemsOptions {
  /** Dynamic spaces to include in results */
  spaces?: Array<{ id: string; name: string; emoji?: string }>
  /** Dynamic bots to include in results */
  bots?: Array<{ id: string; name: string; emoji?: string; description?: string }>
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useCommandItems(options: UseCommandItemsOptions = {}): CommandItem[] {
  const { spaces = [], bots = [] } = options
  const navigate = useNavigate()
  const { theme, setTheme } = useTheme()

  return useMemo(() => {
    const items: CommandItem[] = []

    // ---- Dynamic spaces ----
    for (const space of spaces) {
      items.push({
        id: `space-${space.id}`,
        label: space.name,
        emoji: space.emoji ?? '📂',
        category: 'space',
        keywords: ['space', 'project', 'workspace'],
        action: () => navigate(`/spaces/${space.id}`),
      })
    }

    // ---- Dynamic bots ----
    for (const bot of bots) {
      items.push({
        id: `bot-${bot.id}`,
        label: bot.name,
        description: bot.description,
        emoji: bot.emoji ?? '🤖',
        category: 'bot',
        keywords: ['bot', 'agent', 'reasoner'],
        action: () => navigate(`/reasoners/${bot.id}`),
      })
    }

    // ---- Static pages ----
    const pages: Array<{ label: string; path: string; emoji: string; keywords?: string[] }> = [
      { label: 'Playground', path: '/playground', emoji: '🎮', keywords: ['test', 'try', 'sandbox'] },
      { label: 'Marketplace', path: '/market', emoji: '🏪', keywords: ['market', 'browse', 'templates'] },
      { label: 'Dashboard', path: '/dashboard', emoji: '📊', keywords: ['home', 'overview', 'metrics'] },
      { label: 'Nodes', path: '/nodes', emoji: '🖥️', keywords: ['node', 'server', 'infrastructure'] },
      { label: 'Bots', path: '/reasoners/all', emoji: '🤖', keywords: ['agent', 'reasoner', 'list'] },
      { label: 'Executions', path: '/executions', emoji: '⚡', keywords: ['run', 'execution', 'history'] },
      { label: 'Workflows', path: '/workflows', emoji: '🔄', keywords: ['workflow', 'flow', 'pipeline'] },
      { label: 'DID Explorer', path: '/identity/dids', emoji: '🔑', keywords: ['did', 'identity', 'decentralized'] },
      { label: 'Credentials', path: '/identity/credentials', emoji: '🛡️', keywords: ['credential', 'verifiable', 'vc'] },
      { label: 'Settings', path: '/settings/observability-webhook', emoji: '⚙️', keywords: ['config', 'preferences', 'webhook'] },
    ]

    for (const page of pages) {
      items.push({
        id: `page-${page.path}`,
        label: page.label,
        emoji: page.emoji,
        category: 'page',
        keywords: page.keywords,
        action: () => navigate(page.path),
      })
    }

    // ---- Actions ----
    items.push({
      id: 'action-add-bot',
      label: 'Add New Bot',
      description: 'Create a new bot',
      emoji: '➕',
      category: 'action',
      keywords: ['create', 'new', 'bot', 'add'],
      action: () => {
        // no-op placeholder
      },
    })

    items.push({
      id: 'action-toggle-theme',
      label: 'Toggle Theme',
      description: `Switch to ${theme === 'dark' ? 'light' : 'dark'} mode`,
      emoji: '🌓',
      category: 'action',
      keywords: ['theme', 'dark', 'light', 'mode', 'toggle'],
      action: () => setTheme(theme === 'dark' ? 'light' : 'dark'),
    })

    return items
  }, [navigate, theme, setTheme, spaces, bots])
}
