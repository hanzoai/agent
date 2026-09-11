import type { MemoryScope } from '../types/agent.js';
import type {
  MemoryRequestOptions,
  VectorSearchOptions,
  VectorSearchResult
} from './MemoryClient.js';

/**
 * Where an agent's memory is kept. `MemoryInterface` is what a handler talks
 * to; this holds the bytes. The Go SDK names the same contract `MemoryBackend`.
 */
/**
 * The scope a request addresses and the id within it, resolved the way the
 * control plane's memory handler resolves them. An explicit scope takes its id
 * from the caller or from the execution the request carries; no scope means
 * the narrowest one that execution names, and global otherwise. Global's id is
 * "global" in every SDK, so their records meet.
 */
export function resolveScope(options: MemoryRequestOptions = {}): { scope: MemoryScope; scopeId: string } {
  const { scope, scopeId, metadata } = options;
  const ids: Record<MemoryScope, string | undefined> = {
    workflow: metadata?.workflowId ?? metadata?.runId,
    session: metadata?.sessionId,
    actor: metadata?.actorId,
    global: 'global'
  };
  if (scope) return { scope, scopeId: scopeId ?? ids[scope] ?? '' };
  const named = (['workflow', 'session', 'actor'] as const).find((s) => ids[s]);
  return named ? { scope: named, scopeId: ids[named]! } : { scope: 'global', scopeId: 'global' };
}

export interface MemoryBackend {
  set(key: string, data: any, options?: MemoryRequestOptions): Promise<void>;
  get<T = any>(key: string, options?: MemoryRequestOptions): Promise<T | undefined>;
  delete(key: string, options?: MemoryRequestOptions): Promise<void>;
  listKeys(scope: MemoryScope, options?: MemoryRequestOptions): Promise<string[]>;
  exists(key: string, options?: MemoryRequestOptions): Promise<boolean>;

  setVector(
    key: string,
    embedding: number[],
    metadata?: any,
    options?: MemoryRequestOptions
  ): Promise<void>;
  deleteVector(key: string, options?: MemoryRequestOptions): Promise<void>;
  searchVector(
    queryEmbedding: number[],
    options?: VectorSearchOptions
  ): Promise<VectorSearchResult[]>;
}
