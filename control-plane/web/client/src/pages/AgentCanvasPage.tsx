import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  ReactFlow,
  Background,
  MiniMap,
  Controls,
  useNodesState,
  useEdgesState,
  BackgroundVariant,
  type Node,
  type Edge,
  type NodeTypes,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import {
  Plus,
  Search,
  Grid,
  List,
  ShareNetwork,
  X,
  Play,
  Stop,
  Trash,
  Chat,
  Bot,
  Pulse,
  Users,
  Terminal,
  Eye,
  Maximize,
  Minimize,
  ChevronLeft,
  ChevronRight,
  Monitor,
} from '@/components/ui/icon-bridge';
import { cn } from '@/lib/utils';
import { AgentCanvasProvider, useAgentCanvas } from '@/contexts/AgentCanvasContext';
import { CollaborationProvider } from '@/contexts/CollaborationContext';
import { useCollaboration } from '@/contexts/CollaborationContext';
import { useCollaborativeCanvas } from '@/hooks/useCollaborativeCanvas';
import { useIsMobile } from '@/hooks/use-mobile';
import { CollaborationCursors } from '@/components/agent-canvas/CollaborationCursors';
import { CollaborationBar } from '@/components/agent-canvas/CollaborationBar';
import { StarterNode } from '@/components/agent-canvas/StarterNode';
import { NewAgentModal } from '@/components/agent-canvas/NewAgentModal';
import { AgentListView } from '@/components/agent-canvas/AgentListView';
import type { AgentNodeData, AgentStatus, AgentViewMode } from '@/types/agent-canvas';
import { listAgents as listIdentityAgents } from '@/services/identityApi';
import { pickColor } from '@/lib/collaboration';
import type { CollaborationUser } from '@/types/collaboration';
import { SideChatPanel } from '@/components/chat/SideChatPanel';
import { ChatToggleButton } from '@/components/chat/ChatToggleButton';
import { ShareDialog } from '@/components/collaboration/ShareDialog';
import { TerminalPanel } from '@/components/terminal/TerminalPanel';
import { useSpaces } from '@/contexts/SpaceContext';

// ---------------------------------------------------------------------------
// Team presets
// ---------------------------------------------------------------------------

const TEAM_PRESETS = [
  { id: 'vi', name: 'Vi', emoji: '\u{1F916}', role: 'vi', description: 'Executive assistant & triage', model: 'claude-sonnet-4-5-20250929' },
  { id: 'dev', name: 'Dev', emoji: '\u{1F4BB}', role: 'dev', description: 'Software engineer', model: 'claude-sonnet-4-5-20250929' },
  { id: 'des', name: 'Des', emoji: '\u{1F3A8}', role: 'des', description: 'UI/UX designer', model: 'claude-sonnet-4-5-20250929' },
  { id: 'opera', name: 'Opera', emoji: '\u{1F3D7}\uFE0F', role: 'opera', description: 'DevOps & infrastructure', model: 'claude-sonnet-4-5-20250929' },
  { id: 'su', name: 'Su', emoji: '\u{1F6E1}\uFE0F', role: 'su', description: 'QA & security', model: 'claude-sonnet-4-5-20250929' },
  { id: 'mark', name: 'Mark', emoji: '\u{1F4C8}', role: 'mark', description: 'Marketing & growth', model: 'claude-sonnet-4-5-20250929' },
  { id: 'fin', name: 'Fin', emoji: '\u{1F4B0}', role: 'fin', description: 'Finance & analytics', model: 'claude-sonnet-4-5-20250929' },
  { id: 'art', name: 'Art', emoji: '\u{1F3AD}', role: 'art', description: 'Creative & content', model: 'claude-sonnet-4-5-20250929' },
  { id: 'three', name: 'Three', emoji: '\u{1F9CA}', role: 'three', description: '3D & spatial computing', model: 'claude-sonnet-4-5-20250929' },
  { id: 'fil', name: 'Fil', emoji: '\u{1F3AC}', role: 'fil', description: 'Film & video production', model: 'claude-sonnet-4-5-20250929' },
];

// ---------------------------------------------------------------------------
// AgentNode -- rendered inside ReactFlow
// ---------------------------------------------------------------------------

function AgentNodeCard({ data }: { data: AgentNodeData }) {
  const STATUS_DOT: Record<AgentStatus, string> = {
    running: 'bg-emerald-500 animate-pulse',
    idle: 'bg-emerald-400/70',
    error: 'bg-red-500',
    starting: 'bg-amber-400 animate-pulse',
    stopping: 'bg-amber-400',
  };

  return (
    <div
      className={cn(
        'flex w-[180px] flex-col items-center rounded-xl border border-border/60 bg-card/90 p-4 backdrop-blur-sm',
        'shadow-sm transition-all duration-200',
        'hover:shadow-md hover:border-primary/40'
      )}
    >
      {/* Status indicator */}
      <div className="absolute right-2 top-2">
        <span className={cn('block h-2 w-2 rounded-full', STATUS_DOT[data.status])} />
      </div>

      {/* Emoji */}
      <span className="text-3xl mb-1">{data.emoji}</span>

      {/* Name */}
      <span className="text-sm font-semibold text-foreground">{data.name}</span>

      {/* Role */}
      <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground mt-0.5">
        {data.role}
      </span>

      {/* Status label */}
      <Badge
        variant={
          data.status === 'running'
            ? 'running'
            : data.status === 'error'
              ? 'failed'
              : data.status === 'idle'
                ? 'success'
                : 'pending'
        }
        size="sm"
        className="mt-2"
      >
        {data.status}
      </Badge>
    </div>
  );
}

// Wrap for ReactFlow node type registration
import { memo } from 'react';
import { Handle, Position } from '@xyflow/react';

const AgentNode = memo(({ data }: { data: AgentNodeData }) => (
  <div className="relative">
    <Handle
      type="target"
      position={Position.Left}
      className="!h-2 !w-2 !border-2 !border-border !bg-muted"
    />
    <AgentNodeCard data={data} />
    <Handle
      type="source"
      position={Position.Right}
      className="!h-2 !w-2 !border-2 !border-border !bg-muted"
    />
  </div>
));
AgentNode.displayName = 'AgentNode';

// ---------------------------------------------------------------------------
// View mode type
// ---------------------------------------------------------------------------

type CanvasView = 'canvas' | 'list' | 'grid';
type SessionMode = 'watch' | 'control';

function toAgentStatus(input: string | undefined): AgentStatus {
  switch ((input || '').toLowerCase()) {
    case 'running':
      return 'running';
    case 'starting':
    case 'pending':
      return 'starting';
    case 'stopping':
      return 'stopping';
    case 'error':
    case 'failed':
      return 'error';
    default:
      return 'idle';
  }
}

function buildOrgNodes(agentIds: Array<{ id: string; status?: string }>): { nodes: Node[]; edges: Edge[] } {
  const COLS = 4;
  const GAP_X = 240;
  const GAP_Y = 200;
  const OFFSET_X = 80;
  const OFFSET_Y = 80;

  const nodes: Node[] = agentIds.map((agent, i) => {
    const col = i % COLS;
    const row = Math.floor(i / COLS);
    return {
      id: agent.id,
      type: 'agent',
      position: { x: OFFSET_X + col * GAP_X, y: OFFSET_Y + row * GAP_Y },
      data: {
        id: agent.id,
        name: agent.id,
        emoji: '\u{1F916}',
        role: 'bot',
        model: 'runtime',
        status: toAgentStatus(agent.status),
        stats: { totalExecutions: 0, successCount: 0, failedCount: 0, avgResponseTimeMs: 0 },
        messages: [],
        logs: [],
        createdAt: new Date().toISOString(),
        viewMode: 'chat' as AgentViewMode,
      } satisfies AgentNodeData,
    };
  });

  nodes.push({
    id: 'starter-1',
    type: 'starter',
    position: { x: OFFSET_X + (agentIds.length % COLS) * GAP_X, y: OFFSET_Y + Math.floor(agentIds.length / COLS) * GAP_Y },
    data: { label: 'Add Bot' },
  });

  return { nodes, edges: [] };
}

// ---------------------------------------------------------------------------
// Helper: build initial nodes from presets in a grid layout
// ---------------------------------------------------------------------------

function buildInitialNodes(): { nodes: Node[]; edges: Edge[] } {
  const COLS = 4;
  const GAP_X = 240;
  const GAP_Y = 200;
  const OFFSET_X = 80;
  const OFFSET_Y = 80;

  const nodes: Node[] = TEAM_PRESETS.map((preset, i) => {
    const col = i % COLS;
    const row = Math.floor(i / COLS);
    return {
      id: preset.id,
      type: 'agent',
      position: { x: OFFSET_X + col * GAP_X, y: OFFSET_Y + row * GAP_Y },
      data: {
        id: preset.id,
        name: preset.name,
        emoji: preset.emoji,
        role: preset.role,
        model: preset.model,
        status: 'idle' as AgentStatus,
        stats: { totalExecutions: 0, successCount: 0, failedCount: 0, avgResponseTimeMs: 0 },
        messages: [],
        logs: [],
        createdAt: new Date().toISOString(),
        viewMode: 'chat' as AgentViewMode,
      } satisfies AgentNodeData,
    };
  });

  // Add a starter node at the end of the grid
  const starterCol = TEAM_PRESETS.length % COLS;
  const starterRow = Math.floor(TEAM_PRESETS.length / COLS);
  nodes.push({
    id: 'starter-1',
    type: 'starter',
    position: { x: OFFSET_X + starterCol * GAP_X, y: OFFSET_Y + starterRow * GAP_Y },
    data: { label: 'Add Bot' },
  });

  // Example collaboration edges: Vi talks to Dev and Des
  const edges: Edge[] = [
    { id: 'e-vi-dev', source: 'vi', target: 'dev', animated: true, style: { stroke: 'var(--border)' } },
    { id: 'e-vi-des', source: 'vi', target: 'des', animated: true, style: { stroke: 'var(--border)' } },
    { id: 'e-dev-opera', source: 'dev', target: 'opera', style: { stroke: 'var(--border)' } },
    { id: 'e-dev-su', source: 'dev', target: 'su', style: { stroke: 'var(--border)' } },
  ];

  return { nodes, edges };
}

// ---------------------------------------------------------------------------
// Detail sidebar
// ---------------------------------------------------------------------------

function AgentDetailSidebar({
  agent,
  onClose,
  onStart,
  onStop,
  onDelete,
  onOpenSession,
}: {
  agent: AgentNodeData;
  onClose: () => void;
  onStart: () => void;
  onStop: () => void;
  onDelete: () => void;
  onOpenSession: () => void;
}) {
  return (
    <div className="flex h-full w-[320px] flex-col border-l border-border bg-card">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-border/50 px-4 py-3">
        <div className="flex items-center gap-2">
          <span className="text-xl">{agent.emoji}</span>
          <div>
            <div className="text-sm font-semibold text-foreground">{agent.name}</div>
            <div className="text-[10px] uppercase tracking-wider text-muted-foreground">
              {agent.role}
            </div>
          </div>
        </div>
        <Button variant="ghost" size="icon-sm" onClick={onClose}>
          <X size={16} />
        </Button>
      </div>

      {/* Status */}
      <div className="border-b border-border/50 px-4 py-3 space-y-3">
        <div className="flex items-center justify-between">
          <span className="text-xs text-muted-foreground">Status</span>
          <Badge
            variant={
              agent.status === 'running'
                ? 'running'
                : agent.status === 'error'
                  ? 'failed'
                  : agent.status === 'idle'
                    ? 'success'
                    : 'pending'
            }
            size="sm"
          >
            {agent.status}
          </Badge>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-xs text-muted-foreground">Model</span>
          <span className="text-xs font-mono text-foreground">{agent.model}</span>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-xs text-muted-foreground">Created</span>
          <span className="text-xs text-foreground">
            {new Date(agent.createdAt).toLocaleDateString()}
          </span>
        </div>
      </div>

      {/* Stats */}
      <div className="border-b border-border/50 px-4 py-3">
        <div className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-2">
          Statistics
        </div>
        <div className="grid grid-cols-2 gap-2">
          <div className="rounded-md bg-muted/20 p-2 text-center">
            <div className="text-lg font-semibold text-foreground">
              {agent.stats.totalExecutions}
            </div>
            <div className="text-[10px] text-muted-foreground">Executions</div>
          </div>
          <div className="rounded-md bg-muted/20 p-2 text-center">
            <div className="text-lg font-semibold text-foreground">
              {agent.stats.avgResponseTimeMs > 0
                ? `${(agent.stats.avgResponseTimeMs / 1000).toFixed(1)}s`
                : '--'}
            </div>
            <div className="text-[10px] text-muted-foreground">Avg Time</div>
          </div>
          <div className="rounded-md bg-muted/20 p-2 text-center">
            <div className="text-lg font-semibold text-emerald-500">
              {agent.stats.successCount}
            </div>
            <div className="text-[10px] text-muted-foreground">Success</div>
          </div>
          <div className="rounded-md bg-muted/20 p-2 text-center">
            <div className="text-lg font-semibold text-red-500">
              {agent.stats.failedCount}
            </div>
            <div className="text-[10px] text-muted-foreground">Failed</div>
          </div>
        </div>
      </div>

      {/* Actions */}
      <div className="px-4 py-3 space-y-2 mt-auto">
        {agent.status === 'idle' || agent.status === 'error' ? (
          <Button className="w-full" onClick={onStart}>
            <Play size={14} />
            Start Bot
          </Button>
        ) : (
          <Button className="w-full" variant="outline" onClick={onStop}>
            <Stop size={14} />
            Stop Bot
          </Button>
        )}
        <Button className="w-full" variant="outline">
          <Chat size={14} />
          Open Chat
        </Button>
        <Button className="w-full" variant="outline" onClick={onOpenSession}>
          <Maximize size={14} />
          Open Session
        </Button>
        <Button
          className="w-full"
          variant="destructive"
          onClick={onDelete}
        >
          <Trash size={14} />
          Remove Bot
        </Button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Status summary
// ---------------------------------------------------------------------------

function StatusSummary({ agents }: { agents: AgentNodeData[] }) {
  const running = agents.filter((a) => a.status === 'running').length;
  const idle = agents.filter((a) => a.status === 'idle').length;
  const errored = agents.filter((a) => a.status === 'error').length;

  return (
    <div className="flex items-center gap-3 text-xs">
      {running > 0 && (
        <span className="flex items-center gap-1">
          <span className="h-2 w-2 rounded-full bg-emerald-500 animate-pulse" />
          <span className="text-muted-foreground">
            {running} Running
          </span>
        </span>
      )}
      {idle > 0 && (
        <span className="flex items-center gap-1">
          <span className="h-2 w-2 rounded-full bg-emerald-400/70" />
          <span className="text-muted-foreground">
            {idle} Idle
          </span>
        </span>
      )}
      {errored > 0 && (
        <span className="flex items-center gap-1">
          <span className="h-2 w-2 rounded-full bg-red-500" />
          <span className="text-muted-foreground">
            {errored} Error
          </span>
        </span>
      )}
      {agents.length === 0 && (
        <span className="text-muted-foreground">No bots</span>
      )}
    </div>
  );
}

function LiveStatsOverlay({
  agents,
  peerCount,
  visible,
}: {
  agents: AgentNodeData[];
  peerCount: number;
  visible: boolean;
}) {
  const running = agents.filter((agent) => agent.status === 'running').length;
  const totalExecutions = agents.reduce((sum, agent) => sum + agent.stats.totalExecutions, 0);
  const totalSuccess = agents.reduce((sum, agent) => sum + agent.stats.successCount, 0);
  const totalFailed = agents.reduce((sum, agent) => sum + agent.stats.failedCount, 0);
  const completed = totalSuccess + totalFailed;
  const successRate = completed > 0 ? Math.round((totalSuccess / completed) * 100) : 100;
  const avgLatencySamples = agents
    .map((agent) => agent.stats.avgResponseTimeMs)
    .filter((sample) => sample > 0);
  const avgLatencyMs = avgLatencySamples.length
    ? Math.round(avgLatencySamples.reduce((sum, sample) => sum + sample, 0) / avgLatencySamples.length)
    : 0;

  const cards = [
    { label: 'Running', value: `${running}/${agents.length}`, icon: Pulse },
    { label: 'Success', value: `${successRate}%`, icon: Eye },
    { label: 'Avg latency', value: avgLatencyMs > 0 ? `${(avgLatencyMs / 1000).toFixed(1)}s` : '--', icon: Terminal },
    { label: 'Collaborators', value: `${peerCount + 1}`, icon: Users },
    { label: 'Executions', value: `${totalExecutions}`, icon: Bot },
  ];

  return (
    <div
      className={cn(
        'pointer-events-none absolute inset-x-3 top-3 z-20 transition-all duration-300 md:inset-x-6',
        visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-2'
      )}
    >
      <div className="mx-auto flex max-w-5xl flex-wrap items-center justify-center gap-2">
        {cards.map(({ label, value, icon: Icon }) => (
          <div
            key={label}
            className="flex min-w-[120px] items-center gap-2 rounded-full border border-border/60 bg-card/80 px-3 py-1.5 shadow-sm backdrop-blur-md"
          >
            <Icon size={12} className="text-muted-foreground" />
            <div className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</div>
            <div className="text-xs font-semibold text-foreground">{value}</div>
          </div>
        ))}
      </div>
    </div>
  );
}

function SessionOverlay({
  agent,
  mode,
  onModeChange,
  onClose,
  onPrev,
  onNext,
  canPrev,
  canNext,
  onStart,
  onStop,
}: {
  agent: AgentNodeData;
  mode: SessionMode;
  onModeChange: (mode: SessionMode) => void;
  onClose: () => void;
  onPrev: () => void;
  onNext: () => void;
  canPrev: boolean;
  canNext: boolean;
  onStart: () => void;
  onStop: () => void;
}) {
  const isRunning = agent.status === 'running' || agent.status === 'starting';
  const logTail = agent.logs.slice(-5);
  const terminalLines = [
    `$ hanzo bot connect --id ${agent.id}`,
    `mode=${mode}`,
    `status=${agent.status}`,
    `model=${agent.model}`,
    ...(logTail.length > 0
      ? logTail.map((entry) => `[${entry.level}] ${entry.message}`)
      : ['[info] Waiting for live terminal stream...']),
  ];

  return (
    <div className="fixed inset-0 z-[80] bg-background">
      <div className="flex h-full flex-col">
        <div className="flex items-center justify-between border-b border-border/50 bg-card/90 px-3 py-2 md:px-6">
          <div className="flex min-w-0 items-center gap-2">
            <span className="text-lg">{agent.emoji}</span>
            <div className="min-w-0">
              <div className="truncate text-sm font-semibold text-foreground">{agent.name}</div>
              <div className="truncate text-[11px] text-muted-foreground">{agent.role}</div>
            </div>
            <Badge
              variant={isRunning ? 'running' : agent.status === 'error' ? 'failed' : 'secondary'}
              size="sm"
            >
              {agent.status}
            </Badge>
          </div>

          <div className="flex items-center gap-1 md:gap-2">
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onPrev}
              disabled={!canPrev}
              title="Previous bot"
            >
              <ChevronLeft size={14} />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onNext}
              disabled={!canNext}
              title="Next bot"
            >
              <ChevronRight size={14} />
            </Button>

            <div className="flex items-center rounded-md border border-border/50 bg-muted/30 p-0.5">
              <Button
                variant={mode === 'watch' ? 'secondary' : 'ghost'}
                size="sm"
                className="h-7"
                onClick={() => onModeChange('watch')}
              >
                <Eye size={13} />
                Watch
              </Button>
              <Button
                variant={mode === 'control' ? 'secondary' : 'ghost'}
                size="sm"
                className="h-7"
                onClick={() => onModeChange('control')}
              >
                <Terminal size={13} />
                Control
              </Button>
            </div>

            <Button variant="ghost" size="icon-sm" onClick={onClose} title="Exit fullscreen">
              <Minimize size={14} />
            </Button>
          </div>
        </div>

        <div className="grid flex-1 gap-3 overflow-hidden p-3 md:grid-cols-[2fr_1fr] md:p-6">
          <section className="flex min-h-0 flex-col overflow-hidden rounded-xl border border-border/60 bg-card/50">
            <div className="flex items-center justify-between border-b border-border/40 px-3 py-2 text-xs text-muted-foreground">
              <div className="font-mono">/{agent.id}</div>
              <div>{mode === 'control' ? 'Interactive terminal' : 'Read-only stream'}</div>
            </div>
            <div className="relative flex-1 overflow-auto bg-background px-3 py-3 font-mono text-xs leading-5">
              <div className="absolute inset-0 bg-[radial-gradient(circle_at_top,rgba(34,197,94,0.08),transparent_55%)]" />
              <div className="relative space-y-1 text-foreground/90">
                {terminalLines.map((line, index) => (
                  <div key={`${index}-${line}`}>{line}</div>
                ))}
              </div>
            </div>
          </section>

          <aside className="flex min-h-0 flex-col gap-3 overflow-auto rounded-xl border border-border/60 bg-card/60 p-3">
            <div>
              <div className="text-[10px] uppercase tracking-wide text-muted-foreground">Session Mode</div>
              <p className="mt-1 text-xs text-foreground/90">
                {mode === 'control'
                  ? 'Control mode sends terminal input to the running bot process.'
                  : 'Watch mode is view-only for safe observation.'}
              </p>
            </div>

            <div className="grid grid-cols-2 gap-2">
              <div className="rounded-md border border-border/60 bg-muted/20 p-2">
                <div className="text-[10px] uppercase tracking-wide text-muted-foreground">Executions</div>
                <div className="text-sm font-semibold text-foreground">{agent.stats.totalExecutions}</div>
              </div>
              <div className="rounded-md border border-border/60 bg-muted/20 p-2">
                <div className="text-[10px] uppercase tracking-wide text-muted-foreground">Avg Time</div>
                <div className="text-sm font-semibold text-foreground">
                  {agent.stats.avgResponseTimeMs > 0
                    ? `${(agent.stats.avgResponseTimeMs / 1000).toFixed(1)}s`
                    : '--'}
                </div>
              </div>
            </div>

            {isRunning ? (
              <Button variant="outline" onClick={onStop}>
                <Stop size={14} />
                Stop Bot
              </Button>
            ) : (
              <Button onClick={onStart}>
                <Play size={14} />
                Start Bot
              </Button>
            )}
          </aside>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main canvas content (must be inside AgentCanvasProvider + CollaborationProvider)
// ---------------------------------------------------------------------------

const nodeTypes: NodeTypes = {
  agent: AgentNode,
  starter: StarterNode,
};

function AgentCanvasContent() {
  const { startAgent, stopAgent, deleteAgent } = useAgentCanvas();
  const { connected, joinRoom, peerCount } = useCollaboration();
  const isMobile = useIsMobile();

  const [view, setView] = useState<CanvasView>('canvas');
  const [modalOpen, setModalOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedAgentId, setSelectedAgentId] = useState<string | null>(null);
  const [activeSessionAgentId, setActiveSessionAgentId] = useState<string | null>(null);
  const [sessionMode, setSessionMode] = useState<SessionMode>('watch');
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [statsVisible, setStatsVisible] = useState(true);
  const [orgLoading, setOrgLoading] = useState(false);
  const interactionTimerRef = useRef<number | null>(null);
  const touchStartXRef = useRef<number | null>(null);

  // Build initial layout once
  const initial = useMemo(() => buildInitialNodes(), []);
  const [nodes, setNodes] = useNodesState(initial.nodes);
  const [edges, setEdges] = useEdgesState(initial.edges);

  // Wire collaborative canvas hook — bridges Yjs ↔ ReactFlow
  const collaborative = useCollaborativeCanvas(
    nodes, setNodes,
    edges, setEdges,
    { initialNodes: initial.nodes, initialEdges: initial.edges },
  );

  useEffect(() => {
    let cancelled = false;
    const loadOrgAgents = async () => {
      try {
        setOrgLoading(true);
        const res = await listIdentityAgents(200, 0);
        if (cancelled) return;
        if (res?.agents?.length) {
          const mapped = res.agents.map((a) => ({
            id: a.agent_node_id || a.did || crypto.randomUUID(),
            status: a.status,
          }));
          const next = buildOrgNodes(mapped);
          setNodes(next.nodes);
          setEdges(next.edges);
        }
      } catch {
        // Keep fallback presets if org agent listing fails.
      } finally {
        if (!cancelled) setOrgLoading(false);
      }
    };
    void loadOrgAgents();
    return () => {
      cancelled = true;
    };
  }, [setNodes, setEdges]);

  useEffect(() => {
    if (connected) return;
    const params = new URLSearchParams(window.location.search);
    const room = params.get('room');
    if (!room) return;

    const nameFromStorage = window.localStorage.getItem('hanzo.canvas.displayName') || '';
    const displayName = nameFromStorage || `Guest-${Math.floor(Math.random() * 1000)}`;
    if (!nameFromStorage) {
      window.localStorage.setItem('hanzo.canvas.displayName', displayName);
    }

    const user: CollaborationUser = {
      id: crypto.randomUUID(),
      name: displayName,
      color: pickColor(Math.floor(Math.random() * 16)),
    };
    joinRoom(room, user);
  }, [connected, joinRoom]);

  useEffect(() => {
    return () => {
      if (interactionTimerRef.current) {
        window.clearTimeout(interactionTimerRef.current);
      }
    };
  }, []);

  useEffect(() => {
    document.title = isFullscreen ? 'Playground Session | Hanzo Bot' : 'Playground | Hanzo Bot';
  }, [isFullscreen]);

  const handleInteractionActivity = useCallback(() => {
    setStatsVisible(false);
    if (interactionTimerRef.current) {
      window.clearTimeout(interactionTimerRef.current);
    }
    interactionTimerRef.current = window.setTimeout(() => {
      setStatsVisible(true);
      interactionTimerRef.current = null;
    }, 1200);
  }, []);

  // Collect all agent data from nodes
  const allAgents = useMemo<AgentNodeData[]>(() => {
    return nodes
      .filter((n) => n.type === 'agent')
      .map((n) => n.data as unknown as AgentNodeData);
  }, [nodes]);

  // Filtered agents for search
  const filteredAgents = useMemo(() => {
    if (!searchQuery.trim()) return allAgents;
    const q = searchQuery.toLowerCase();
    return allAgents.filter(
      (a) =>
        a.name.toLowerCase().includes(q) ||
        a.role.toLowerCase().includes(q) ||
        a.model.toLowerCase().includes(q)
    );
  }, [allAgents, searchQuery]);

  const selectedAgent = useMemo(
    () => allAgents.find((a) => a.id === selectedAgentId) ?? null,
    [allAgents, selectedAgentId]
  );

  const activeSessionAgent = useMemo(
    () => allAgents.find((agent) => agent.id === activeSessionAgentId) ?? null,
    [allAgents, activeSessionAgentId]
  );

  useEffect(() => {
    if (activeSessionAgentId && !activeSessionAgent) {
      setIsFullscreen(false);
      setActiveSessionAgentId(null);
    }
  }, [activeSessionAgent, activeSessionAgentId]);

  const handleNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      if (node.type === 'agent') {
        setSelectedAgentId(node.id);
      }
    },
    []
  );

  const handlePaneClick = useCallback(() => {
    setSelectedAgentId(null);
  }, []);

  const openAgentSession = useCallback(
    (agentId: string) => {
      setActiveSessionAgentId(agentId);
      setIsFullscreen(true);
      handleInteractionActivity();
    },
    [handleInteractionActivity]
  );

  const handleSessionNavigate = useCallback(
    (direction: -1 | 1) => {
      if (!activeSessionAgentId || allAgents.length <= 1) return;
      const currentIndex = allAgents.findIndex((agent) => agent.id === activeSessionAgentId);
      if (currentIndex < 0) return;
      const nextIndex = currentIndex + direction;
      if (nextIndex < 0 || nextIndex >= allAgents.length) return;
      setActiveSessionAgentId(allAgents[nextIndex].id);
    },
    [activeSessionAgentId, allAgents]
  );

  const handleTouchStart = useCallback((event: React.TouchEvent) => {
    touchStartXRef.current = event.touches[0]?.clientX ?? null;
  }, []);

  const handleTouchEnd = useCallback(
    (event: React.TouchEvent) => {
      if (!isFullscreen || !activeSessionAgentId) return;
      const startX = touchStartXRef.current;
      const endX = event.changedTouches[0]?.clientX ?? null;
      touchStartXRef.current = null;
      if (startX === null || endX === null) return;
      const deltaX = endX - startX;
      if (Math.abs(deltaX) < 48) return;
      handleSessionNavigate(deltaX < 0 ? 1 : -1);
    },
    [activeSessionAgentId, handleSessionNavigate, isFullscreen]
  );

  const handleAgentCreated = useCallback(
    (agent: AgentNodeData) => {
      const COLS = 4;
      const GAP_X = 240;
      const GAP_Y = 200;
      const OFFSET_X = 80;
      const OFFSET_Y = 80;
      const count = nodes.filter((n) => n.type === 'agent').length;
      const col = count % COLS;
      const row = Math.floor(count / COLS);

      const newNode: Node = {
        id: agent.id,
        type: 'agent',
        position: { x: OFFSET_X + col * GAP_X, y: OFFSET_Y + row * GAP_Y },
        data: agent as unknown as Record<string, unknown>,
      };

      // Use collaborative addAgentNode so it propagates to all peers
      collaborative.addAgentNode(newNode);
    },
    [nodes, collaborative]
  );

  const activeSessionIndex = useMemo(
    () => allAgents.findIndex((agent) => agent.id === activeSessionAgentId),
    [allAgents, activeSessionAgentId]
  );
  const canSessionPrev = activeSessionIndex > 0;
  const canSessionNext = activeSessionIndex >= 0 && activeSessionIndex < allAgents.length - 1;
  const sessionLaunchTarget = selectedAgent ?? filteredAgents[0] ?? null;

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <div className="flex h-full w-full flex-col">
      {/* Top toolbar */}
      <div className="flex flex-wrap items-center justify-between border-b border-border/50 bg-card/80 px-3 py-2 backdrop-blur-sm md:px-4 gap-2 flex-shrink-0">
        {/* Left: Add + View toggle */}
        <div className="flex items-center gap-2 min-w-0">
          <Button size="sm" onClick={() => setModalOpen(true)}>
            <Plus size={14} />
            Add Bot
          </Button>

          <div className="flex items-center rounded-md border border-border/50 bg-muted/30 p-0.5">
            <Button
              variant={view === 'canvas' ? 'secondary' : 'ghost'}
              size="icon-sm"
              onClick={() => setView('canvas')}
              title="Spatial view"
            >
              <ShareNetwork size={14} />
            </Button>
            <Button
              variant={view === 'list' ? 'secondary' : 'ghost'}
              size="icon-sm"
              onClick={() => setView('list')}
              title="List view"
            >
              <List size={14} />
            </Button>
            <Button
              variant={view === 'grid' ? 'secondary' : 'ghost'}
              size="icon-sm"
              onClick={() => setView('grid')}
              title="Grid view"
            >
              <Grid size={14} />
            </Button>
          </div>

          <Button
            size="sm"
            variant="outline"
            className={cn(isMobile && 'px-2')}
            disabled={!sessionLaunchTarget}
            onClick={() => sessionLaunchTarget && openAgentSession(sessionLaunchTarget.id)}
          >
            <Maximize size={14} />
            {!isMobile && 'Open Session'}
          </Button>
        </div>

        {/* Center: Status summary */}
        <div className="flex items-center gap-3">
          <StatusSummary agents={allAgents} />
          {orgLoading && <span className="text-xs text-muted-foreground">Loading org bots...</span>}
        </div>

        {/* Right: Collaboration + Search */}
        <div className="flex items-center gap-2">
          <CollaborationBar />
          <div className={cn('relative', isMobile ? 'w-40' : 'w-56')}>
            <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="Search bots..."
              className="h-7 pl-8 text-xs"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
          </div>
        </div>
      </div>

      {/* Main content area */}
      <div
        className="relative flex flex-1 min-h-0"
        onMouseMove={handleInteractionActivity}
        onWheel={handleInteractionActivity}
        onTouchStart={(event) => {
          handleInteractionActivity();
          handleTouchStart(event);
        }}
        onTouchEnd={handleTouchEnd}
      >
        <LiveStatsOverlay
          agents={allAgents}
          peerCount={peerCount}
          visible={!isFullscreen && statsVisible}
        />

        {/* Canvas view */}
        {view === 'canvas' && (
          <div
            className="flex-1 relative"
            onMouseMove={(event) => {
              collaborative.onMouseMove(event);
              handleInteractionActivity();
            }}
            onMouseLeave={collaborative.onMouseLeave}
          >
            {/* Remote cursors overlay */}
            <CollaborationCursors />

            <ReactFlow
              nodes={nodes}
              edges={edges}
              onNodesChange={collaborative.onNodesChange}
              onEdgesChange={collaborative.onEdgesChange}
              onConnect={collaborative.onConnect}
              onNodeClick={handleNodeClick}
              onPaneClick={handlePaneClick}
              nodeTypes={nodeTypes}
              fitView
              fitViewOptions={{ padding: 0.2 }}
              proOptions={{ hideAttribution: true }}
              className="bg-background"
            >
              <Background
                variant={BackgroundVariant.Dots}
                gap={20}
                size={1}
                color="var(--border)"
              />
              <MiniMap
                position="bottom-right"
                className="!bg-card/80 !border-border/50 !rounded-lg !shadow-sm"
                maskColor="rgba(0, 0, 0, 0.15)"
                nodeStrokeWidth={2}
              />
              <Controls
                position="bottom-left"
                className="!bg-card !border-border/50 !rounded-lg !shadow-sm [&>button]:!bg-card [&>button]:!border-border/40 [&>button]:!text-foreground [&>button]:hover:!bg-muted"
              />
            </ReactFlow>
          </div>
        )}

        {/* List view */}
        {view === 'list' && (
          <div className="flex-1 overflow-y-auto p-4">
            <AgentListView
              agents={filteredAgents}
              onSelect={(a) => setSelectedAgentId(a.id)}
              onStart={(id) => startAgent(id)}
              onStop={(id) => stopAgent(id)}
              onChat={(id) => openAgentSession(id)}
              onDelete={(id) => deleteAgent(id)}
            />
          </div>
        )}

        {/* Grid view */}
        {view === 'grid' && (
          <div className="flex-1 overflow-y-auto p-4">
            {filteredAgents.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-20 text-center">
                <Bot size={24} className="text-muted-foreground mb-2" />
                <p className="text-sm text-muted-foreground">No bots match your search.</p>
              </div>
            ) : (
              <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
                {filteredAgents.map((agent) => (
                  <div
                    key={agent.id}
                    className="flex cursor-pointer flex-col items-start gap-2"
                    onClick={() => setSelectedAgentId(agent.id)}
                    onDoubleClick={() => openAgentSession(agent.id)}
                  >
                    <AgentNodeCard data={agent} />
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-7 w-[180px] text-xs"
                      onClick={(event) => {
                        event.stopPropagation();
                        openAgentSession(agent.id);
                      }}
                    >
                      <Monitor size={13} />
                      Open Session
                    </Button>
                  </div>
                ))}
                {/* Starter card */}
                <div
                  className={cn(
                    'flex h-[140px] w-[180px] cursor-pointer flex-col items-center justify-center',
                    'rounded-xl border-2 border-dashed border-border/60 bg-card/40',
                    'transition-all duration-200 hover:border-primary/50 hover:bg-card/70'
                  )}
                  onClick={() => setModalOpen(true)}
                >
                  <Plus size={20} className="text-muted-foreground" />
                  <span className="mt-2 text-xs font-medium text-muted-foreground">
                    Add Bot
                  </span>
                </div>
              </div>
            )}
          </div>
        )}

        {/* Right sidebar detail panel */}
        {selectedAgent && (
          <AgentDetailSidebar
            agent={selectedAgent}
            onClose={() => setSelectedAgentId(null)}
            onStart={() => startAgent(selectedAgent.id)}
            onStop={() => stopAgent(selectedAgent.id)}
            onDelete={() => {
              deleteAgent(selectedAgent.id);
              setSelectedAgentId(null);
              collaborative.removeNode(selectedAgent.id);
            }}
            onOpenSession={() => openAgentSession(selectedAgent.id)}
          />
        )}
      </div>

      {isFullscreen && activeSessionAgent && (
        <SessionOverlay
          agent={activeSessionAgent}
          mode={sessionMode}
          onModeChange={setSessionMode}
          onClose={() => setIsFullscreen(false)}
          onPrev={() => handleSessionNavigate(-1)}
          onNext={() => handleSessionNavigate(1)}
          canPrev={canSessionPrev}
          canNext={canSessionNext}
          onStart={() => startAgent(activeSessionAgent.id)}
          onStop={() => stopAgent(activeSessionAgent.id)}
        />
      )}

      {/* New agent modal */}
      <NewAgentModal
        open={modalOpen}
        onOpenChange={setModalOpen}
        onAgentCreated={handleAgentCreated}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Exported page (wrapped in providers)
// ---------------------------------------------------------------------------

export function AgentCanvasPage() {
  return (
    <CollaborationProvider>
      <AgentCanvasProvider>
        <AgentCanvasContent />
      </AgentCanvasProvider>
    </CollaborationProvider>
  );
}
