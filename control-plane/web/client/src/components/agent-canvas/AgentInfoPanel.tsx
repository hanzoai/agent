import {
  Play,
  Square,
  Clock,
  Activity,
  CheckCircle2,
  XCircle,
  Cpu,
  Wallet,
  Fingerprint,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { ScrollArea } from '@/components/ui/scroll-area'
import { CopyButton } from '@/components/ui/copy-button'
import { cn } from '@/lib/utils'
import type { AgentInstance, AgentStatus } from '@/types/agent-canvas'

interface AgentInfoPanelProps {
  agent: AgentInstance
  onStart: () => void
  onStop: () => void
}

const STATUS_CONFIG: Record<
  AgentStatus,
  { label: string; color: string; dotColor: string }
> = {
  running: {
    label: 'Running',
    color: 'text-emerald-400',
    dotColor: 'bg-emerald-400',
  },
  idle: {
    label: 'Idle',
    color: 'text-muted-foreground',
    dotColor: 'bg-muted-foreground',
  },
  error: {
    label: 'Error',
    color: 'text-red-400',
    dotColor: 'bg-red-400',
  },
  starting: {
    label: 'Starting',
    color: 'text-blue-400',
    dotColor: 'bg-blue-400',
  },
  stopping: {
    label: 'Stopping',
    color: 'text-amber-400',
    dotColor: 'bg-amber-400',
  },
}

function truncate(value: string | undefined, len: number = 16): string {
  if (!value) return '-'
  if (value.length <= len) return value
  const half = Math.floor((len - 3) / 2)
  return `${value.slice(0, half)}...${value.slice(-half)}`
}

function StatItem({
  icon,
  label,
  value,
  className,
}: {
  icon: React.ReactNode
  label: string
  value: string | number
  className?: string
}) {
  return (
    <div className="flex items-center justify-between gap-2 text-xs">
      <span className="flex items-center gap-1.5 text-muted-foreground">
        {icon}
        {label}
      </span>
      <span className={cn('font-mono font-medium text-foreground', className)}>
        {value}
      </span>
    </div>
  )
}

export function AgentInfoPanel({ agent, onStart, onStop }: AgentInfoPanelProps) {
  const cfg = STATUS_CONFIG[agent.status]
  const isActive = agent.status === 'running' || agent.status === 'starting'

  const formatMs = (ms: number): string => {
    if (ms < 1000) return `${Math.round(ms)}ms`
    return `${(ms / 1000).toFixed(1)}s`
  }

  const formatDate = (iso: string | undefined): string => {
    if (!iso) return '-'
    return new Date(iso).toLocaleString([], {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  return (
    <ScrollArea className="h-full">
      <div className="flex flex-col gap-3 p-3 text-sm">
        {/* Status */}
        <div className="flex items-center justify-between">
          <span className="text-xs text-muted-foreground">Status</span>
          <div className="flex items-center gap-1.5">
            <span
              className={cn('h-2 w-2 rounded-full', cfg.dotColor, {
                'animate-pulse': agent.status === 'running' || agent.status === 'starting',
              })}
            />
            <span className={cn('text-xs font-medium', cfg.color)}>
              {cfg.label}
            </span>
          </div>
        </div>

        {/* Role & Model */}
        <div className="flex items-center justify-between">
          <span className="text-xs text-muted-foreground">Role</span>
          <span className="text-xs font-medium text-foreground">{agent.role}</span>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-xs text-muted-foreground">Model</span>
          <span className="text-xs font-mono text-foreground">{agent.model}</span>
        </div>

        <Separator className="my-1" />

        {/* Identity */}
        <div className="space-y-2">
          <h4 className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
            Identity
          </h4>

          {/* DID */}
          <div className="flex items-center justify-between gap-1">
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <Fingerprint className="h-3 w-3" />
              <span>DID</span>
            </div>
            <div className="flex items-center gap-1">
              <span className="text-xs font-mono text-foreground">
                {truncate(agent.did, 20)}
              </span>
              {agent.did && <CopyButton value={agent.did} size="icon-sm" className="h-5 w-5" />}
            </div>
          </div>

          {/* Wallet */}
          <div className="flex items-center justify-between gap-1">
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <Wallet className="h-3 w-3" />
              <span>Wallet</span>
            </div>
            <div className="flex items-center gap-1">
              <span className="text-xs font-mono text-foreground">
                {truncate(agent.wallet, 20)}
              </span>
              {agent.wallet && <CopyButton value={agent.wallet} size="icon-sm" className="h-5 w-5" />}
            </div>
          </div>
        </div>

        <Separator className="my-1" />

        {/* Execution Stats */}
        <div className="space-y-2">
          <h4 className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
            Execution Stats
          </h4>
          <StatItem
            icon={<Activity className="h-3 w-3" />}
            label="Total"
            value={agent.stats.totalExecutions}
          />
          <StatItem
            icon={<CheckCircle2 className="h-3 w-3" />}
            label="Success"
            value={agent.stats.successCount}
            className="text-emerald-400"
          />
          <StatItem
            icon={<XCircle className="h-3 w-3" />}
            label="Failed"
            value={agent.stats.failedCount}
            className={agent.stats.failedCount > 0 ? 'text-red-400' : undefined}
          />
          <StatItem
            icon={<Cpu className="h-3 w-3" />}
            label="Avg Response"
            value={formatMs(agent.stats.avgResponseTimeMs)}
          />
        </div>

        <Separator className="my-1" />

        {/* Timestamps */}
        <div className="space-y-2">
          <div className="flex items-center justify-between text-xs">
            <span className="flex items-center gap-1.5 text-muted-foreground">
              <Clock className="h-3 w-3" />
              Created
            </span>
            <span className="font-mono text-foreground">
              {formatDate(agent.createdAt)}
            </span>
          </div>
          <div className="flex items-center justify-between text-xs">
            <span className="flex items-center gap-1.5 text-muted-foreground">
              <Clock className="h-3 w-3" />
              Last Active
            </span>
            <span className="font-mono text-foreground">
              {formatDate(agent.lastActive)}
            </span>
          </div>
        </div>

        <Separator className="my-1" />

        {/* Actions */}
        <div className="flex items-center gap-2 pt-1">
          <Button
            size="sm"
            variant={isActive ? 'secondary' : 'default'}
            className="flex-1 gap-1.5"
            onClick={isActive ? onStop : onStart}
            disabled={agent.status === 'starting' || agent.status === 'stopping'}
          >
            {isActive ? (
              <>
                <Square className="h-3 w-3" />
                Stop
              </>
            ) : (
              <>
                <Play className="h-3 w-3" />
                Start
              </>
            )}
          </Button>
        </div>
      </div>
    </ScrollArea>
  )
}
