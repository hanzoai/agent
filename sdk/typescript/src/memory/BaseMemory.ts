import axios, { AxiosInstance } from 'axios';
import type { MemoryScope } from '../types/agent.js';
import { httpAgent, httpsAgent } from '../utils/httpAgents.js';
import { resolveScope, type MemoryBackend } from './MemoryBackend.js';
import type {
  MemoryRequestOptions,
  VectorSearchOptions,
  VectorSearchResult
} from './MemoryClient.js';

/**
 * Agent memory kept in Hanzo Base, one record per scope and key in one
 * collection. Import `agent_memory.collection.json` from the Go SDK once; the
 * Go, TypeScript and Python stores share it.
 */
export class BaseMemory implements MemoryBackend {
  private readonly http: AxiosInstance;
  private readonly collection: string;

  constructor(baseUrl: string, token: string, collection = 'agent_memory') {
    this.http = axios.create({
      baseURL: baseUrl.replace(/\/$/, ''),
      timeout: 30000,
      httpAgent,
      httpsAgent,
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      // Every status is taken, so `ask` can name an unrouted path's HTML answer.
      validateStatus: () => true
    });
    this.collection = collection;
  }

  private get records() {
    return `/v1/collections/${encodeURIComponent(this.collection)}/records`;
  }

  async set(key: string, data: any, options: MemoryRequestOptions = {}) {
    await this.write(options, key, { value: JSON.stringify(data) });
  }

  async get<T = any>(key: string, options: MemoryRequestOptions = {}): Promise<T | undefined> {
    const record = await this.find(options, key);
    if (!record?.value) return undefined;
    return JSON.parse(record.value) as T;
  }

  /** Absent is not an error: the caller asked for the key to be gone and it is. */
  async delete(key: string, options: MemoryRequestOptions = {}) {
    const record = await this.find(options, key);
    if (!record) return;
    await this.ask('delete', `${this.records}/${encodeURIComponent(record.id)}`);
  }

  /** Every key in a scope, across all of Base's pages. */
  async listKeys(scope: MemoryScope, options: MemoryRequestOptions = {}) {
    const keys: string[] = [];
    for (let page = 1; ; page++) {
      const answer = await this.ask('get', this.records, {
        perPage: 200,
        page,
        filter: filterFor(scope, resolveScope({ ...options, scope }).scopeId)
      });
      const items = answer.items ?? [];
      keys.push(...items.map((item: any) => item.mkey).filter(Boolean));
      if (!items.length || keys.length >= (answer.totalItems ?? 0)) return keys;
    }
  }

  async exists(key: string, options: MemoryRequestOptions = {}) {
    return (await this.get(key, options)) !== undefined;
  }

  async setVector(key: string, embedding: number[], metadata?: any, options: MemoryRequestOptions = {}) {
    await this.write(options, key, { embedding, metadata: metadata ?? null });
  }

  /** Clears the embedding and keeps any value stored at the key. */
  async deleteVector(key: string, options: MemoryRequestOptions = {}) {
    const record = await this.find(options, key);
    if (!record) return;
    await this.ask('patch', `${this.records}/${encodeURIComponent(record.id)}`, undefined, {
      embedding: null,
      metadata: null
    });
  }

  /**
   * Ranks the scope's vectors by cosine similarity, in the client: cost grows
   * with the scope, which suits an agent's working set, not a corpus.
   */
  async searchVector(
    queryEmbedding: number[],
    options: VectorSearchOptions = {}
  ): Promise<VectorSearchResult[]> {
    if (!queryEmbedding?.length) throw new Error('search needs a query vector');

    const { scope, scopeId: id } = resolveScope(options);
    const found: VectorSearchResult[] = [];

    for (let page = 1; ; page++) {
      const answer = await this.ask('get', this.records, {
        perPage: 200,
        page,
        filter: filterFor(scope, id)
      });
      const items = answer.items ?? [];
      if (!items.length) break;

      for (const item of items) {
        // A different width is a different embedding model; skip it.
        if (item.embedding?.length !== queryEmbedding.length) continue;
        if (!matches(item.metadata, options.filters)) continue;
        found.push({
          key: item.mkey,
          scope: item.scope,
          scopeId: item.scope_id,
          score: cosine(queryEmbedding, item.embedding),
          metadata: item.metadata ?? undefined
        });
      }
      if (items.length >= (answer.totalItems ?? 0)) break;
    }

    found.sort((a, b) => b.score - a.score);
    return found.slice(0, options.topK ?? 10);
  }

  /** The one record at a key, or undefined where there is none. */
  private async find(options: MemoryRequestOptions, key: string) {
    const answer = await this.ask('get', this.records, {
      perPage: 1,
      filter: filterFor(...where(options), key)
    });
    return answer.items?.[0];
  }

  /**
   * Creates or updates the one record at a key.
   *
   * PATCH, not PUT: setting a value must not erase an embedding written beside
   * it, and the two share a record.
   */
  private async write(options: MemoryRequestOptions, key: string, fields: Record<string, any>) {
    const { scope, scopeId: id } = resolveScope(options);
    const existing = await this.find(options, key);

    if (existing) {
      await this.ask('patch', `${this.records}/${encodeURIComponent(existing.id)}`, undefined, fields);
      return;
    }
    try {
      await this.ask('post', this.records, undefined, { scope, scope_id: id, mkey: key, ...fields });
    } catch (err) {
      // Another writer created the key between the read and this create, and
      // the unique index refused a second record. A set means the last write
      // wins, so update the record that won.
      const winner = await this.find(options, key);
      if (!winner) throw err;
      await this.ask('patch', `${this.records}/${encodeURIComponent(winner.id)}`, undefined, fields);
    }
  }

  private async ask(
    method: 'get' | 'post' | 'patch' | 'delete',
    path: string,
    params?: Record<string, any>,
    body?: any
  ) {
    const answer = await this.http.request({ method, url: path, params, data: body });

    // Base answers a path it does not route with its admin SPA, at 200.
    if (String(answer.headers['content-type'] ?? '').startsWith('text/html')) {
      throw new Error(`base answered with HTML for ${path} — that path is not served by this Base`);
    }
    if (answer.status >= 300) {
      throw new Error(`base returned ${answer.status} for ${path}: ${snippet(answer.data)}`);
    }
    return answer.data ?? {};
  }
}

/** The scope and id a request addresses, in the order a filter names them. */
function where(options: MemoryRequestOptions): [MemoryScope, string] {
  const { scope, scopeId } = resolveScope(options);
  return [scope, scopeId];
}

/** A clause in Base's filter grammar, with every value quoted. */
function filterFor(scope: MemoryScope, id: string, key?: string) {
  const parts = [`scope=${quote(scope)}`, `scope_id=${quote(id)}`];
  if (key !== undefined) parts.push(`mkey=${quote(key)}`);
  return parts.join(' && ');
}

/**
 * A single-quoted literal. Base's tokenizer reads a backslash as an escape, so
 * backslashes double before quotes are escaped: the other order lets a value
 * ending in one escape the closing quote.
 */
function quote(value: string) {
  return `'${value.replace(/\\/g, '\\\\').replace(/'/g, "\\'")}'`;
}

/** 1 for the same direction, 0 for perpendicular. A zero vector has none. */
function cosine(a: number[], b: number[]) {
  let dot = 0;
  let na = 0;
  let nb = 0;
  for (let i = 0; i < a.length; i++) {
    dot += a[i] * b[i];
    na += a[i] * a[i];
    nb += b[i] * b[i];
  }
  if (!na || !nb) return 0;
  return dot / (Math.sqrt(na) * Math.sqrt(nb));
}

/** Whether metadata satisfies every filter; a missing key fails its filter. */
function matches(metadata: Record<string, any> | null | undefined, filters?: Record<string, any>) {
  if (!filters) return true;
  for (const [key, want] of Object.entries(filters)) {
    if (metadata?.[key] !== want) return false;
  }
  return true;
}

/**
 * Enough of a body to identify a refusal, and not enough to put a token or a
 * stored value into a log.
 */
function snippet(body: any) {
  const text = typeof body === 'string' ? body : JSON.stringify(body ?? '');
  return text.length > 200 ? `${text.slice(0, 200)}…` : text;
}
