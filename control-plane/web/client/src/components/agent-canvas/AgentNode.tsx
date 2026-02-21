import { memo, useCallback } from 'react'
import { Handle, Position } from '@xyflow/react'
import {
  MessageSquare,
  Terminal,
  ScrollText,
  Info,
  Play,
  Square,
  Trash2,
  Loader2,
} from 'lucide-react'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { useAgentCanvas } from '@/contexts/AgentCanvasContext'
import { AgentChatView } from '@/components/agent-canvas/AgentChatView'
import { AgentInfoPanel } from '@/components/agent-canvas/AgentInfoPanel'
import type { AgentNodeData, AgentStatus, AgentViewMode, LogEntry } from '@/types/agent-canvas'

/** Props passed by ReactFlow to custom node components */
interface AgentNodeComponentProps {
  id: string
  data: AgentNodeData
  selected?: boolean
}

// ---------------------------------------------------------------------------
// Status styling
// ---------------------------------------------------------------------------

const STATUS_DOT: Record<AgentStatus, string> = {
  running: 'bg-emerald-400 shadow-emerald-400/50 shadow-[0_0_6px]',
  idle: 'bg-muted-foreground',
  error: 'bg-red-400 shadow-red-400/50 shadow-[0_0_6px]',
  starting: 'bg-blue-400 animate-pulse',
  stopping: 'bg-amber-400 animate-pulse',
}

const STATUS_BADGE_VARIANT: Record<AgentStatus, string> = {
  running: 'running',
  idle: 'secondary',
  error: 'failed',
  starting: 'pending',
  stopping: 'pending',
}

const STATUS_LABEL: Record<AgentStatus, string> = {
  running: 'Running',
  idle: 'Idle',
  error: 'Error',
  starting: 'Starting',
  stopping: 'Stopping',
}

// ---------------------------------------------------------------------------
// Log level colors
// ---------------------------------------------------------------------------

const LOG_LEVEL_COLOR: Record<string, string> = {
  info: 'text-blue-400',
  warn: 'text-amber-400',
  error: 'text-red-400',
  debug: 'text-muted-foreground',
}

// ---------------------------------------------------------------------------
// Tab config
// ---------------------------------------------------------------------------

const VIEW_TABS: { value: AgentViewMode; icon: typeof MessageSquare; label: string }[] = [
  { value: 'chat', icon: MessageSquare, label: 'Chat' },
  { value: 'terminal', icon: Terminal, label: 'Terminal' },
  { value: 'logs', icon: ScrollText, label: 'Logs' },
  { value: 'info', icon: Info, label: 'Info' },
]

// ---------------------------------------------------------------------------
// Logs sub-view (inline, small enough to not warrant its own file)
// ---------------------------------------------------------------------------

function LogsView({ logs }: { logs: LogEntry[] }) {
  if (logs.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 py-10 text-muted-foreground">
        <ScrollText className="h-6 w-6 opacity-40" />
        <p className="text-xs">No log entries yet</p>
      </div>
    )
  }

  return (
    <ScrollArea className="h-full">
      <div className="flex flex-col gap-0.5 p-2 font-mono text-[11px] leading-relaxed">
        {logs.map((entry) => (
          <div key={entry.id} className="flex gap-2">
            <span className="flex-shrink-0 text-muted-foreground/60">
              {new Date(entry.timestamp).toLocaleTimeString([], {
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit',
              })}
            </span>
            <span
              className={cn(
                'flex-shrink-0 w-[38px] uppercase font-semibold',
                LOG_LEVEL_COLOR[entry.level] ?? 'text-muted-foreground'
              )}
            >
              {entry.level}
            </span>
            {entry.source && (
              <span className="flex-shrink-0 text-muted-foreground/60">
                [{entry.source}]
              </span>
            )}
            <span className="text-foreground break-all">{entry.message}</span>
          </div>
        ))}
      </div>
    </ScrollArea>
  )
}

// ---------------------------------------------------------------------------
// Terminal placeholder
// ---------------------------------------------------------------------------

function TerminalView() {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 bg-background/40 rounded-md">
      <Terminal className="h-6 w-6 text-muted-foreground/40" />
      <p className="text-xs text-muted-foreground">Terminal connecting...</p>
      <Loader2 className="h-3 w-3 animate-spin text-muted-foreground/60" />
    </div>
  )
}

// ---------------------------------------------------------------------------
// AgentNode
// ---------------------------------------------------------------------------

export const AgentNode = memo(
  ({ data, id }: AgentNodeComponentProps) => {
    const { sendMessage, startAgent, stopAgent, deleteAgent, setViewMode } =
      useAgentCanvas()

    const isActive =
      data.status === 'running' || data.status === 'starting'

    // Handlers
    const handleSendMessage = useCallback(
      (content: string) => sendMessage(id, content),
      [sendMessage, id]
    )
    const handleStart = useCallback(() => startAgent(id), [startAgent, id])
    const handleStop = useCallback(() => stopAgent(id), [stopAgent, id])
    const handleDelete = useCallback(() => deleteAgent(id), [deleteAgent, id])
    const handleTabChange = useCallback(
      (value: string) => setViewMode(id, value as AgentViewMode),
      [setViewMode, id]
    )

    return (
      <>
        {/* Source/target handles for ReactFlow connections */}
        <Handle
          type="target"
          position={Position.Left}
          className="!w-2.5 !h-2.5 !bg-muted-foreground/40 !border-2 !border-background hover:!bg-primary transition-colors"
          id="target"
        />
        <Handle
          type="source"
          position={Position.Right}
          className="!w-2.5 !h-2.5 !bg-muted-foreground/40 !border-2 !border-background hover:!bg-primary transition-colors"
          id="source"
        />

        <Card
          variant="default"
          interactive={false}
          className={cn(
            'w-[400px] min-h-[320px] overflow-hidden',
            'resize overflow-auto',
            'border-border/60 bg-card/95 backdrop-blur-md',
            'shadow-lg',
            data.status === 'error' && 'border-red-500/40',
            data.status === 'running' && 'border-emerald-500/20'
          )}
          style={{ minWidth: 320, maxWidth: 600 }}
        >
          {/* ---- Header ---- */}
          <div className="flex items-center justify-between border-b border-border/40 px-3 py-2">
            {/* Left: emoji + name + role + status dot */}
            <div className="flex items-center gap-2 min-w-0">
              <span className="text-lg leading-none flex-shrink-0" role="img" aria-label={data.name}>
                {data.emoji}
              </span>
              <div className="flex flex-col min-w-0">
                <div className="flex items-center gap-1.5">
                  <span className="text-sm font-semibold text-foreground truncate">
                    {data.name}
                  </span>
                  <span
                    className={cn(
                      'h-2 w-2 rounded-full flex-shrink-0',
                      STATUS_DOT[data.status]
                    )}
                    title={STATUS_LABEL[data.status]}
                  />
                </div>
                <span className="text-[11px] text-muted-foreground truncate">
                  {data.role}
                </span>
              </div>
            </div>

            {/* Right: status badge + toolbar */}
            <div className="flex items-center gap-1.5 flex-shrink-0">
              <Badge
                variant={STATUS_BADGE_VARIANT[data.status] as any}
                size="sm"
                showIcon={false}
              >
                {STATUS_LABEL[data.status]}
              </Badge>

              {/* Toolbar */}
              <div className="flex items-center gap-0.5 ml-1">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      onClick={isActive ? handleStop : handleStart}
                      disabled={
                        data.status === 'starting' || data.status === 'stopping'
                      }
                      aria-label={isActive ? 'Stop agent' : 'Start agent'}
                    >
                      {isActive ? (
                        <Square className="h-3.5 w-3.5" />
                      ) : (
                        <Play className="h-3.5 w-3.5" />
                      )}
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side="bottom">
                    {isActive ? 'Stop' : 'Start'}
                  </TooltipContent>
                </Tooltip>

                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      onClick={handleDelete}
                      className="text-muted-foreground hover:text-destructive"
                      aria-label="Delete agent"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side="bottom">Delete</TooltipContent>
                </Tooltip>
              </div>
            </div>
          </div>

          {/* ---- Tabbed content ---- */}
          <Tabs
            value={data.viewMode}
            onValueChange={handleTabChange}
            className="flex flex-col flex-1"
          >
            <TabsList variant="underline" className="px-3 pt-1">
              {VIEW_TABS.map(({ value, icon: Icon, label }) => (
                <TabsTrigger
                  key={value}
                  value={value}
                  variant="underline"
                  size="sm"
                  className="gap-1"
                >
                  <Icon className="h-3 w-3" />
                  {label}
                </TabsTrigger>
              ))}
            </TabsList>

            {/* Fixed height content area */}
            <div className="h-[240px]">
              <TabsContent value="chat" className="h-full mt-0">
                <AgentChatView
                  agent={data}
                  messages={data.messages}
                  isStreaming={data.isStreaming}
                  onSendMessage={handleSendMessage}
                />
              </TabsContent>

              <TabsContent value="terminal" className="h-full mt-0">
                <TerminalView />
              </TabsContent>

              <TabsContent value="logs" className="h-full mt-0">
                <LogsView logs={data.logs} />
              </TabsContent>

              <TabsContent value="info" className="h-full mt-0">
                <AgentInfoPanel
                  agent={data}
                  onStart={handleStart}
                  onStop={handleStop}
                />
              </TabsContent>
            </div>
          </Tabs>
        </Card>
      </>
    )
  },
  (prev, next) => {
    // Custom memo comparison for performance
    const pd = prev.data as AgentNodeData
    const nd = next.data as AgentNodeData
    return (
      pd.id === nd.id &&
      pd.status === nd.status &&
      pd.viewMode === nd.viewMode &&
      pd.isStreaming === nd.isStreaming &&
      pd.messages.length === nd.messages.length &&
      pd.logs.length === nd.logs.length &&
      pd.name === nd.name &&
      pd.role === nd.role &&
      pd.stats.totalExecutions === nd.stats.totalExecutions &&
      pd.stats.successCount === nd.stats.successCount &&
      pd.stats.failedCount === nd.stats.failedCount
    )
  }
)

AgentNode.displayName = 'AgentNode'

// ---------------------------------------------------------------------------
// NodeTypes export for ReactFlow registration
// ---------------------------------------------------------------------------

export const agentNodeTypes = {
  agentNode: AgentNode,
} as const
