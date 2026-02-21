import { memo, useState } from 'react';
import { Handle, Position } from '@xyflow/react';
import { Plus } from '@/components/ui/icon-bridge';
import { cn } from '@/lib/utils';
import { NewAgentModal } from './NewAgentModal';

interface StarterNodeData {
  label?: string;
}

interface StarterNodeProps {
  data: StarterNodeData;
}

export const StarterNode = memo(({ data }: StarterNodeProps) => {
  const [modalOpen, setModalOpen] = useState(false);

  return (
    <>
      <div
        className={cn(
          'flex h-[100px] w-[140px] cursor-pointer flex-col items-center justify-center',
          'rounded-xl border-2 border-dashed border-border/60 bg-card/40 backdrop-blur-sm',
          'transition-all duration-200',
          'hover:border-primary/50 hover:bg-card/70 hover:shadow-md',
          'active:scale-[0.97]'
        )}
        onClick={() => setModalOpen(true)}
      >
        <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-muted/50 transition-colors group-hover:bg-muted">
          <Plus size={20} className="text-muted-foreground" />
        </div>
        <span className="mt-2 text-xs font-medium text-muted-foreground">
          {data.label || 'Add Bot'}
        </span>

        {/* Invisible handles for potential edge connections */}
        <Handle
          type="target"
          position={Position.Left}
          className="!h-1 !w-1 !border-0 !bg-transparent !opacity-0"
        />
        <Handle
          type="source"
          position={Position.Right}
          className="!h-1 !w-1 !border-0 !bg-transparent !opacity-0"
        />
      </div>

      <NewAgentModal open={modalOpen} onOpenChange={setModalOpen} />
    </>
  );
});

StarterNode.displayName = 'StarterNode';
