import type { MemoryScope } from '../types/agent.js';
import type {
  MemoryRequestOptions,
  VectorSearchOptions,
  VectorSearchResult
} from './MemoryClient.js';

/**
 * Where an agent's memory is kept.
 *
 * `MemoryInterface` is what a handler talks to; this is what holds the bytes.
 * It named one class before, so a second store could not be used without
 * editing the facade — and a class with private fields is not structurally
 * assignable from another, so having the same methods was not enough.
 *
 * The same name as the Go SDK's `MemoryBackend`, for the same concept. One
 * name per idea across the SDKs is what makes them one SDK in several
 * languages rather than several SDKs.
 */
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
