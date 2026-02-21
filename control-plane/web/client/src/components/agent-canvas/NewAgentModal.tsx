import { useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ArrowLeft, Plus, Settings } from '@/components/ui/icon-bridge';
import { cn } from '@/lib/utils';
import type { AgentNodeData, AgentViewMode } from '@/types/agent-canvas';

// The 10 Hanzo team presets
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
] as const;

const AVAILABLE_MODELS = [
  'claude-sonnet-4-5-20250929',
  'claude-opus-4-20250514',
  'gpt-4o',
  'gpt-4o-mini',
  'qwen3-235b',
  'qwen3-30b',
];

type Step = 'select' | 'configure';

interface PresetSelection {
  id: string;
  name: string;
  emoji: string;
  role: string;
  description: string;
  model: string;
}

interface NewAgentModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onAgentCreated?: (agent: AgentNodeData) => void;
}

export function NewAgentModal({ open, onOpenChange, onAgentCreated }: NewAgentModalProps) {
  const [step, setStep] = useState<Step>('select');
  const [selected, setSelected] = useState<PresetSelection | null>(null);

  // Configuration form state
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [model, setModel] = useState('claude-sonnet-4-5-20250929');
  const [gatewayUrl, setGatewayUrl] = useState('');

  const resetForm = () => {
    setStep('select');
    setSelected(null);
    setName('');
    setDescription('');
    setModel('claude-sonnet-4-5-20250929');
    setGatewayUrl('');
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) resetForm();
    onOpenChange(nextOpen);
  };

  const handlePresetSelect = (preset: PresetSelection) => {
    setSelected(preset);
    setName(preset.name);
    setDescription(preset.description);
    setModel(preset.model);
    setStep('configure');
  };

  const handleCustomSelect = () => {
    setSelected({
      id: 'custom',
      name: '',
      emoji: '\u{2699}\uFE0F',
      role: 'custom',
      description: '',
      model: 'claude-sonnet-4-5-20250929',
    });
    setName('');
    setDescription('');
    setStep('configure');
  };

  const handleCreate = () => {
    if (!name.trim()) return;

    const agent: AgentNodeData = {
      id: `agent-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      name: name.trim(),
      emoji: selected?.emoji ?? '\u{2699}\uFE0F',
      role: selected?.role ?? 'custom',
      model,
      status: 'idle',
      stats: {
        totalExecutions: 0,
        successCount: 0,
        failedCount: 0,
        avgResponseTimeMs: 0,
      },
      messages: [],
      logs: [],
      createdAt: new Date().toISOString(),
      viewMode: 'chat' as AgentViewMode,
    };

    onAgentCreated?.(agent);
    handleOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {step === 'select' ? 'New Bot' : 'Configure Bot'}
          </DialogTitle>
          <DialogDescription>
            {step === 'select'
              ? 'Choose a team preset or create a custom bot.'
              : `Configure ${selected?.name || 'your new bot'} before launching.`}
          </DialogDescription>
        </DialogHeader>

        {step === 'select' && (
          <div className="space-y-4">
            {/* Preset grid */}
            <div className="grid grid-cols-5 gap-2">
              {TEAM_PRESETS.map((preset) => (
                <button
                  key={preset.id}
                  type="button"
                  onClick={() => handlePresetSelect(preset)}
                  className={cn(
                    'flex flex-col items-center gap-1 rounded-lg border border-border/60 bg-card p-3',
                    'transition-all duration-150 hover:border-primary/50 hover:bg-accent hover:shadow-sm',
                    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
                  )}
                >
                  <span className="text-2xl">{preset.emoji}</span>
                  <span className="text-xs font-medium text-foreground">{preset.name}</span>
                </button>
              ))}
            </div>

            {/* Custom agent option */}
            <button
              type="button"
              onClick={handleCustomSelect}
              className={cn(
                'flex w-full items-center gap-3 rounded-lg border border-dashed border-border/60 bg-card/50 p-3',
                'transition-all duration-150 hover:border-primary/50 hover:bg-accent',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
              )}
            >
              <div className="flex h-8 w-8 items-center justify-center rounded-md bg-muted">
                <Settings size={16} className="text-muted-foreground" />
              </div>
              <div className="text-left">
                <div className="text-sm font-medium text-foreground">Custom Bot</div>
                <div className="text-xs text-muted-foreground">
                  Configure from scratch with your own settings
                </div>
              </div>
            </button>
          </div>
        )}

        {step === 'configure' && (
          <div className="space-y-4">
            {/* Back button */}
            <button
              type="button"
              onClick={() => setStep('select')}
              className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
            >
              <ArrowLeft size={14} />
              Back to presets
            </button>

            {/* Preview */}
            <div className="flex items-center gap-3 rounded-lg border border-border/40 bg-muted/20 p-3">
              <span className="text-3xl">{selected?.emoji}</span>
              <div>
                <div className="text-sm font-semibold text-foreground">
                  {name || 'Unnamed Agent'}
                </div>
                <div className="text-xs text-muted-foreground">
                  {selected?.role ?? 'custom'}
                </div>
              </div>
            </div>

            {/* Name */}
            <div className="space-y-1.5">
              <Label htmlFor="agent-name">Name</Label>
              <Input
                id="agent-name"
                placeholder="Agent name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                autoFocus
              />
            </div>

            {/* Description */}
            <div className="space-y-1.5">
              <Label htmlFor="agent-description">Description</Label>
              <Input
                id="agent-description"
                placeholder="What does this agent do?"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </div>

            {/* Model */}
            <div className="space-y-1.5">
              <Label>Model</Label>
              <Select value={model} onValueChange={setModel}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {AVAILABLE_MODELS.map((m) => (
                    <SelectItem key={m} value={m}>
                      {m}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Gateway URL (optional) */}
            <div className="space-y-1.5">
              <Label htmlFor="agent-gateway">Gateway URL (optional)</Label>
              <Input
                id="agent-gateway"
                placeholder="https://..."
                value={gatewayUrl}
                onChange={(e) => setGatewayUrl(e.target.value)}
              />
            </div>
          </div>
        )}

        {step === 'configure' && (
          <DialogFooter>
            <Button variant="outline" onClick={() => handleOpenChange(false)}>
              Cancel
            </Button>
            <Button onClick={handleCreate} disabled={!name.trim()}>
              <Plus size={14} />
              Create Bot
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
