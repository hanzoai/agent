import { useState, useMemo } from 'react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Play,
  Stop,
  Chat,
  Trash,
  CaretUp,
  CaretDown,
  Bot,
} from '@/components/ui/icon-bridge';
import { cn } from '@/lib/utils';
import type { AgentNodeData, AgentStatus } from '@/types/agent-canvas';

type SortField = 'name' | 'role' | 'model' | 'totalExecutions' | 'lastActive' | 'status';
type SortOrder = 'asc' | 'desc';

interface AgentListViewProps {
  agents: AgentNodeData[];
  onSelect?: (agent: AgentNodeData) => void;
  onStart?: (agentId: string) => void;
  onStop?: (agentId: string) => void;
  onChat?: (agentId: string) => void;
  onDelete?: (agentId: string) => void;
}

const STATUS_DOT_COLOR: Record<AgentStatus, string> = {
  running: 'bg-emerald-500',
  idle: 'bg-emerald-400/70',
  error: 'bg-red-500',
  starting: 'bg-amber-400',
  stopping: 'bg-amber-400',
};

function SortableHeader({
  label,
  field,
  sortBy,
  sortOrder,
  onSort,
  className,
}: {
  label: string;
  field: SortField;
  sortBy: SortField;
  sortOrder: SortOrder;
  onSort: (field: SortField) => void;
  className?: string;
}) {
  const isActive = sortBy === field;
  return (
    <button
      type="button"
      onClick={() => onSort(field)}
      className={cn(
        'flex items-center gap-1 text-xs font-medium uppercase tracking-wide',
        'text-muted-foreground hover:text-foreground transition-colors',
        className
      )}
    >
      <span>{label}</span>
      <span className="flex flex-col leading-none">
        <CaretUp
          size={10}
          className={cn(
            isActive && sortOrder === 'asc' ? 'text-primary' : 'text-muted-foreground/40'
          )}
        />
        <CaretDown
          size={10}
          className={cn(
            '-mt-0.5',
            isActive && sortOrder === 'desc' ? 'text-primary' : 'text-muted-foreground/40'
          )}
        />
      </span>
    </button>
  );
}

export function AgentListView({
  agents,
  onSelect,
  onStart,
  onStop,
  onChat,
  onDelete,
}: AgentListViewProps) {
  const [sortBy, setSortBy] = useState<SortField>('name');
  const [sortOrder, setSortOrder] = useState<SortOrder>('asc');

  const handleSort = (field: SortField) => {
    if (sortBy === field) {
      setSortOrder((prev) => (prev === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortBy(field);
      setSortOrder('asc');
    }
  };

  const sorted = useMemo(() => {
    const copy = [...agents];
    copy.sort((a, b) => {
      let cmp = 0;
      switch (sortBy) {
        case 'name':
          cmp = a.name.localeCompare(b.name);
          break;
        case 'role':
          cmp = a.role.localeCompare(b.role);
          break;
        case 'model':
          cmp = a.model.localeCompare(b.model);
          break;
        case 'totalExecutions':
          cmp = a.stats.totalExecutions - b.stats.totalExecutions;
          break;
        case 'lastActive':
          cmp = (a.lastActive ?? '').localeCompare(b.lastActive ?? '');
          break;
        case 'status':
          cmp = a.status.localeCompare(b.status);
          break;
      }
      return sortOrder === 'asc' ? cmp : -cmp;
    });
    return copy;
  }, [agents, sortBy, sortOrder]);

  if (agents.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-center">
        <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-muted/50 mb-3">
          <Bot size={24} className="text-muted-foreground" />
        </div>
        <h3 className="text-sm font-medium text-foreground">No bots</h3>
        <p className="mt-1 text-xs text-muted-foreground">
          Create a bot to get started.
        </p>
      </div>
    );
  }

  const gridTemplate = '32px 1.5fr 0.8fr 1fr 0.8fr 1fr 140px';

  return (
    <div className="rounded-lg border border-border/50 bg-card shadow-sm overflow-hidden">
      {/* Header */}
      <div className="border-b border-border/50 bg-muted/30 px-4 py-2">
        <div className="grid items-center gap-2" style={{ gridTemplateColumns: gridTemplate }}>
          <SortableHeader label="" field="status" sortBy={sortBy} sortOrder={sortOrder} onSort={handleSort} />
          <SortableHeader label="Name" field="name" sortBy={sortBy} sortOrder={sortOrder} onSort={handleSort} />
          <SortableHeader label="Role" field="role" sortBy={sortBy} sortOrder={sortOrder} onSort={handleSort} />
          <SortableHeader label="Model" field="model" sortBy={sortBy} sortOrder={sortOrder} onSort={handleSort} />
          <SortableHeader label="Executions" field="totalExecutions" sortBy={sortBy} sortOrder={sortOrder} onSort={handleSort} />
          <SortableHeader label="Last Active" field="lastActive" sortBy={sortBy} sortOrder={sortOrder} onSort={handleSort} />
          <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Actions
          </div>
        </div>
      </div>

      {/* Rows */}
      <div className="divide-y divide-border/30">
        {sorted.map((agent) => (
          <div
            key={agent.id}
            className={cn(
              'grid items-center gap-2 px-4 py-2.5 transition-colors duration-150',
              'hover:bg-muted/20 cursor-pointer'
            )}
            style={{ gridTemplateColumns: gridTemplate }}
            onClick={() => onSelect?.(agent)}
          >
            {/* Status dot */}
            <div className="flex items-center justify-center">
              <span
                className={cn(
                  'h-2.5 w-2.5 rounded-full',
                  STATUS_DOT_COLOR[agent.status],
                  agent.status === 'running' && 'animate-pulse'
                )}
              />
            </div>

            {/* Name */}
            <div className="flex items-center gap-2 min-w-0">
              <span className="text-base flex-shrink-0">{agent.emoji}</span>
              <span className="text-sm font-medium text-foreground truncate">{agent.name}</span>
            </div>

            {/* Role */}
            <div>
              <Badge variant="secondary" size="sm">{agent.role}</Badge>
            </div>

            {/* Model */}
            <div className="text-xs font-mono text-muted-foreground truncate">
              {agent.model.replace('claude-sonnet-4-5-20250929', 'sonnet-4.5').replace('claude-opus-4-20250514', 'opus-4')}
            </div>

            {/* Executions */}
            <div className="text-xs font-mono text-muted-foreground">
              {agent.stats.totalExecutions}
            </div>

            {/* Last Active */}
            <div className="text-xs text-muted-foreground">
              {agent.lastActive
                ? new Date(agent.lastActive).toLocaleTimeString('en-US', {
                    hour12: false,
                    hour: '2-digit',
                    minute: '2-digit',
                  })
                : '--'}
            </div>

            {/* Actions */}
            <div className="flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
              {agent.status === 'idle' || agent.status === 'error' ? (
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => onStart?.(agent.id)}
                  title="Start"
                >
                  <Play size={14} />
                </Button>
              ) : (
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => onStop?.(agent.id)}
                  title="Stop"
                >
                  <Stop size={14} />
                </Button>
              )}
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => onChat?.(agent.id)}
                title="Chat"
              >
                <Chat size={14} />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => onDelete?.(agent.id)}
                title="Delete"
                className="text-muted-foreground hover:text-destructive"
              >
                <Trash size={14} />
              </Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
