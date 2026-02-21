import { useEffect, useRef, useState, useCallback } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import { cn } from '@/lib/utils'
import { X, Terminal as TerminalIcon, Eye } from '@/components/ui/icon-bridge'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface TerminalPanelProps {
  agentId: string
  agentName?: string
  mode: 'watch' | 'control'
  className?: string
  onClose?: () => void
}

type ConnectionStatus = 'connecting' | 'connected' | 'disconnected'

// ---------------------------------------------------------------------------
// Theme
// ---------------------------------------------------------------------------

const TERMINAL_THEME = {
  background: '#0a0a0f',
  foreground: '#e5e7eb',
  cursor: '#22c55e',
  cursorAccent: '#0a0a0f',
  selectionBackground: '#374151',
  black: '#1f2937',
  red: '#ef4444',
  green: '#22c55e',
  yellow: '#eab308',
  blue: '#3b82f6',
  magenta: '#a855f7',
  cyan: '#06b6d4',
  white: '#e5e7eb',
} as const

const FONT_FAMILY = "'JetBrains Mono', 'Fira Code', 'Menlo', monospace"
const FONT_SIZE = 13

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function buildWsUrl(agentId: string): string {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${window.location.host}/api/ui/v1/agents/${agentId}/terminal`
}

function writeWelcome(term: Terminal, agentName: string | undefined, agentId: string, mode: string) {
  const label = agentName || agentId
  term.writeln(`\x1b[32m\u25cf\x1b[0m Connected to bot: ${label}`)
  term.writeln(`\x1b[90mMode: ${mode} | Press Ctrl+C to interrupt\x1b[0m`)
  term.writeln('')
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function TerminalPanel({
  agentId,
  agentName,
  mode,
  className,
  onClose,
}: TerminalPanelProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const [status, setStatus] = useState<ConnectionStatus>('connecting')

  // ---- fit helper (stable reference) ----
  const fit = useCallback(() => {
    try {
      fitAddonRef.current?.fit()
    } catch {
      // Container may not be visible yet; swallow.
    }
  }, [])

  // ---- main lifecycle ----
  useEffect(() => {
    const el = containerRef.current
    if (!el) return

    // -- Terminal setup --
    const fitAddon = new FitAddon()
    const webLinksAddon = new WebLinksAddon()

    const term = new Terminal({
      theme: TERMINAL_THEME,
      fontFamily: FONT_FAMILY,
      fontSize: FONT_SIZE,
      cursorBlink: true,
      cursorStyle: 'bar',
      allowProposedApi: true,
      disableStdin: mode === 'watch',
      convertEol: true,
      scrollback: 5000,
    })

    term.loadAddon(fitAddon)
    term.loadAddon(webLinksAddon)
    term.open(el)

    termRef.current = term
    fitAddonRef.current = fitAddon

    // Initial fit after DOM paint
    requestAnimationFrame(() => fit())

    // -- Welcome message --
    writeWelcome(term, agentName, agentId, mode)

    // -- WebSocket --
    let ws: WebSocket | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null

    function connect() {
      setStatus('connecting')
      try {
        ws = new WebSocket(buildWsUrl(agentId))
      } catch {
        setStatus('disconnected')
        term.writeln('\x1b[31mTerminal not available - agent may not be running\x1b[0m')
        return
      }

      ws.binaryType = 'arraybuffer'

      ws.onopen = () => {
        setStatus('connected')
        wsRef.current = ws
      }

      ws.onmessage = (ev: MessageEvent) => {
        if (typeof ev.data === 'string') {
          term.write(ev.data)
        } else if (ev.data instanceof ArrayBuffer) {
          term.write(new Uint8Array(ev.data))
        }
      }

      ws.onerror = () => {
        // onerror always fires before onclose; let onclose handle state.
      }

      ws.onclose = () => {
        setStatus('disconnected')
        wsRef.current = null
        term.writeln('')
        term.writeln('\x1b[31mTerminal not available - agent may not be running\x1b[0m')
      }
    }

    connect()

    // -- Input handling (control mode only) --
    let dataDisposable: { dispose(): void } | null = null
    if (mode === 'control') {
      dataDisposable = term.onData((data: string) => {
        const currentWs = wsRef.current
        if (currentWs && currentWs.readyState === WebSocket.OPEN) {
          currentWs.send(data)
        } else {
          // Echo locally when no connection (development aid)
          term.write(data)
        }
      })
    }

    // -- Resize observer --
    const observer = new ResizeObserver(() => fit())
    observer.observe(el)

    // -- Cleanup --
    return () => {
      observer.disconnect()
      dataDisposable?.dispose()
      if (reconnectTimer) clearTimeout(reconnectTimer)
      if (ws) {
        ws.onopen = null
        ws.onmessage = null
        ws.onerror = null
        ws.onclose = null
        ws.close()
      }
      wsRef.current = null
      fitAddonRef.current = null
      term.dispose()
      termRef.current = null
    }
  }, [agentId, mode]) // eslint-disable-line react-hooks/exhaustive-deps
  // agentName intentionally excluded: changing display name should not remount.

  // ---- render ----
  return (
    <div
      className={cn(
        'flex flex-col overflow-hidden rounded-lg border border-border bg-[#0a0a0f]',
        className,
      )}
    >
      {/* ---- Header ---- */}
      <div className="flex items-center justify-between gap-2 border-b border-border/60 bg-bg-secondary px-3 py-1.5">
        <div className="flex items-center gap-2 min-w-0">
          <TerminalIcon size={14} className="flex-shrink-0 text-text-secondary" />
          <span className="truncate text-xs font-medium text-text-primary">
            {agentName || agentId}
          </span>
          <Badge
            variant={mode === 'watch' ? 'running' : 'success'}
            size="sm"
            showIcon={false}
          >
            {mode === 'watch' ? (
              <span className="flex items-center gap-1">
                <Eye size={10} /> watch
              </span>
            ) : (
              <span className="flex items-center gap-1">
                <TerminalIcon size={10} /> control
              </span>
            )}
          </Badge>
          <StatusDot status={status} />
        </div>
        {onClose && (
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onClose}
            aria-label="Close terminal"
          >
            <X size={14} />
          </Button>
        )}
      </div>

      {/* ---- Terminal viewport ---- */}
      <div
        ref={containerRef}
        className="flex-1 min-h-0"
        style={{ padding: 4 }}
      />
    </div>
  )
}

// ---------------------------------------------------------------------------
// StatusDot - tiny connection indicator
// ---------------------------------------------------------------------------

function StatusDot({ status }: { status: ConnectionStatus }) {
  const color =
    status === 'connected'
      ? 'bg-green-500'
      : status === 'connecting'
        ? 'bg-yellow-500 animate-pulse'
        : 'bg-red-500'

  const label =
    status === 'connected'
      ? 'Connected'
      : status === 'connecting'
        ? 'Connecting...'
        : 'Disconnected'

  return (
    <span className="flex items-center gap-1 text-[10px] text-text-secondary" title={label}>
      <span className={cn('inline-block h-1.5 w-1.5 rounded-full', color)} />
      {label}
    </span>
  )
}

export default TerminalPanel
