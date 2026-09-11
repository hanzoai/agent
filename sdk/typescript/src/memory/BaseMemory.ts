import axios, { AxiosInstance } from 'axios';
import type { MemoryScope } from '../types/agent.js';
import { httpAgent, httpsAgent } from '../utils/httpAgents.js';
import type { MemoryBackend } from './MemoryBackend.js';
import type {
  MemoryRequestOptions,
  VectorSearchOptions,
  VectorSearchResult
} from './MemoryClient.js';

/**
 * Agent memory kept in Hanzo Base.
 *
 * Base is the single-binary Go backend, so an agent's state sits in the same
 * process tree as everything it runs beside: no second database, no separate
 * migration, and the records are visible in Base's own admin view while the
 * agent is running.
 *
 * One record per scope and key, in one collection. A collection per scope
 * would make the number of collections a function of how many workflows have
 * ever run.
 *
 * The collection must exist, carrying the fields this writes. Import
 * `agent_memory.collection.json` from the Go SDK once — the schema is the same
 * one, because the two SDKs read each other's records. It is not created on
 * first write: a client that builds its own schema can build the wrong one
 * from a typo and then read nothing from the collection everyone else is
 * looking at.
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
      // Base's router answers a path it does not serve with the admin SPA, at
      // 200. Taking every status lets `read` say that, rather than throwing an
      // error that reads like the server is broken.
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

  /**
   * Every key in a scope, following Base's pages to the end. A caller asking
   * for all the keys and receiving the first page would read that as the whole
   * answer.
   */
  async listKeys(scope: MemoryScope, options: MemoryRequestOptions = {}) {
    const keys: string[] = [];
    for (let page = 1; ; page++) {
      const answer = await this.ask('get', this.records, {
        perPage: 200,
        page,
        filter: filterFor(scope, scopeId(options))
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

  /**
   * The embedding, not the record: a value may be stored at the same key and
   * is not the caller's to lose here.
   */
  async deleteVector(key: string, options: MemoryRequestOptions = {}) {
    const record = await this.find(options, key);
    if (!record) return;
    await this.ask('patch', `${this.records}/${encodeURIComponent(record.id)}`, undefined, {
      embedding: null,
      metadata: null
    });
  }

  /**
   * Ranks the scope's vectors by cosine similarity.
   *
   * Scored here rather than in the database, and that is a limit worth
   * stating: it reads the scope's vectors and ranks them in the client, so
   * cost grows with the size of the scope. Right for an agent's own working
   * set, wrong for a corpus. Base can score server-side through a hook, and a
   * scope large enough to need that should use one.
   */
  async searchVector(
    queryEmbedding: number[],
    options: VectorSearchOptions = {}
  ): Promise<VectorSearchResult[]> {
    if (!queryEmbedding?.length) throw new Error('search needs a query vector');

    const scope = options.scope ?? 'workflow';
    const id = scopeId(options);
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
        // A vector of a different width came from a different embedding model.
        // Cosine across two of those returns a number that means nothing, so
        // it is skipped rather than scored.
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
      filter: filterFor(options.scope ?? 'workflow', scopeId(options), key)
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
    const scope = options.scope ?? 'workflow';
    const id = scopeId(options);
    const existing = await this.find(options, key);

    if (existing) {
      await this.ask('patch', `${this.records}/${encodeURIComponent(existing.id)}`, undefined, fields);
      return;
    }
    await this.ask('post', this.records, undefined, {
      scope,
      scope_id: id,
      mkey: key,
      ...fields
    });
  }

  private async ask(
    method: 'get' | 'post' | 'patch' | 'delete',
    path: string,
    params?: Record<string, any>,
    body?: any
  ) {
    const answer = await this.http.request({ method, url: path, params, data: body });

    // An HTML body on a JSON endpoint means the path was not served and the
    // SPA catch-all answered. Saying so beats a parse error, which reads as a
    // broken server rather than a wrong collection.
    if (String(answer.headers['content-type'] ?? '').startsWith('text/html')) {
      throw new Error(`base answered with HTML for ${path} — that path is not served by this Base`);
    }
    if (answer.status >= 300) {
      throw new Error(`base returned ${answer.status} for ${path}: ${snippet(answer.data)}`);
    }
    return answer.data ?? {};
  }
}

/** A scope id, defaulting to the empty string so a filter clause is well formed. */
function scopeId(options: MemoryRequestOptions) {
  return options.scopeId ?? '';
}

/** A clause in Base's filter grammar, with every value quoted. */
function filterFor(scope: MemoryScope, id: string, key?: string) {
  const parts = [`scope=${quote(scope)}`, `scope_id=${quote(id)}`];
  if (key !== undefined) parts.push(`mkey=${quote(key)}`);
  return parts.join(' && ');
}

/**
 * A single-quoted literal, escaped.
 *
 * Base's tokenizer treats a backslash as the escape rune, so a quote preceded
 * by one does not close the literal. That makes the order load-bearing:
 * backslashes double first, then quotes. The other order lets a value ending
 * in a backslash escape the closing quote and end the literal where the caller
 * chose. A scope id is caller data.
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

/**
 * Whether a record's metadata satisfies every filter. A filter naming a key
 * the record lacks excludes it: the caller asked for records where that key
 * holds a value, and a record without it is not one.
 */
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
