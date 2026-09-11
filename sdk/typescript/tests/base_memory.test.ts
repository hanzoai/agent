import { describe, expect, it, afterEach } from 'vitest';
import http from 'node:http';
import { BaseMemory } from '../src/memory/BaseMemory.js';

/**
 * A Base standing in for the real one, over the same records API. Enough of it
 * to hold this store to its contract offline: the filter grammar it sends, the
 * paging it walks, and the create-or-patch decision it makes.
 */
class FakeBase {
  records = new Map<string, any>();
  private next = 0;
  private server?: http.Server;

  async listen() {
    this.server = http.createServer((req, res) => this.route(req, res));
    await new Promise<void>((done) => this.server!.listen(0, '127.0.0.1', done));
    const address = this.server!.address() as { port: number };
    return `http://127.0.0.1:${address.port}`;
  }

  async close() {
    if (this.server) await new Promise<void>((done) => this.server!.close(() => done()));
  }

  private async route(req: http.IncomingMessage, res: http.ServerResponse) {
    const url = new URL(req.url!, 'http://base');
    const prefix = '/v1/collections/agent_memory/records';

    // The admin SPA a real Base serves for a path it does not route: HTML, at
    // 200. It is here because that is the failure this store has to name.
    if (!url.pathname.startsWith(prefix)) {
      res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' });
      res.end('<!doctype html><title>Base</title>');
      return;
    }

    const id = url.pathname.slice(prefix.length).replace(/^\//, '');
    const body = await read(req);

    if (req.method === 'GET' && !id) return json(res, this.list(url));
    if (req.method === 'POST' && !id) return json(res, this.create(body), 201);
    if (req.method === 'PATCH' && id) return json(res, this.patch(id, body));
    if (req.method === 'DELETE' && id) {
      this.records.delete(id);
      res.writeHead(204).end();
      return;
    }
    res.writeHead(405).end();
  }

  private list(url: URL) {
    const want: Record<string, string> = {};
    for (const [, field, value] of (url.searchParams.get('filter') ?? '').matchAll(
      /(\w+)='((?:[^'\\]|\\.)*)'/g
    )) {
      want[field] = unescape(value);
    }

    let hits = [...this.records.values()].filter(
      (rec) =>
        rec.scope === want.scope &&
        rec.scope_id === want.scope_id &&
        (want.mkey === undefined || rec.mkey === want.mkey)
    );
    hits.sort((a, b) => (a.mkey < b.mkey ? -1 : 1));

    const total = hits.length;
    const perPage = Number(url.searchParams.get('perPage') ?? 30);
    const page = Number(url.searchParams.get('page') ?? 1);
    const from = Math.min((page - 1) * perPage, total);
    return { items: hits.slice(from, from + perPage), totalItems: total };
  }

  private create(body: any) {
    const record = { ...body, id: `r${++this.next}` };
    this.records.set(record.id, record);
    return record;
  }

  /** A merge, which is what PATCH means: an absent field keeps its value. */
  private patch(id: string, body: any) {
    const record = this.records.get(id);
    if (!record) return {};
    for (const [field, value] of Object.entries(body ?? {})) record[field] = value;
    return record;
  }
}

/** Undo the escaping a quoted literal carries: a backslash escapes what follows. */
function unescape(value: string) {
  let out = '';
  for (let i = 0; i < value.length; i++) {
    if (value[i] === '\\' && i + 1 < value.length) i++;
    out += value[i];
  }
  return out;
}

function read(req: http.IncomingMessage): Promise<any> {
  return new Promise((done) => {
    let raw = '';
    req.on('data', (chunk) => (raw += chunk));
    req.on('end', () => {
      try {
        done(raw ? JSON.parse(raw) : undefined);
      } catch {
        done(undefined);
      }
    });
  });
}

function json(res: http.ServerResponse, payload: any, status = 200) {
  res.writeHead(status, { 'content-type': 'application/json' });
  res.end(JSON.stringify(payload));
}

let fake: FakeBase | undefined;

async function backedByFake(collection = 'agent_memory') {
  fake = new FakeBase();
  const url = await fake.listen();
  return new BaseMemory(url, 'test-token', collection);
}

afterEach(async () => {
  await fake?.close();
  fake = undefined;
});

describe('BaseMemory', () => {
  it('reads back what it wrote', async () => {
    const memory = await backedByFake();
    await memory.set('greeting', 'hello', { scope: 'session', scopeId: 's1' });
    expect(await memory.get('greeting', { scope: 'session', scopeId: 's1' })).toBe('hello');
  });

  it('keeps the shape of a structured value', async () => {
    const memory = await backedByFake();
    const plan = { steps: ['read', 'write'], depth: 2 };
    await memory.set('plan', plan, { scope: 'workflow', scopeId: 'w1' });
    expect(await memory.get('plan', { scope: 'workflow', scopeId: 'w1' })).toEqual(plan);
  });

  it('replaces rather than appending, so one key is one record', async () => {
    const memory = await backedByFake();
    await memory.set('tone', 'terse', { scope: 'actor', scopeId: 'u1' });
    await memory.set('tone', 'plain', { scope: 'actor', scopeId: 'u1' });

    expect(await memory.get('tone', { scope: 'actor', scopeId: 'u1' })).toBe('plain');
    expect(fake!.records.size).toBe(1);
  });

  it('answers undefined for a key never written', async () => {
    const memory = await backedByFake();
    expect(await memory.get('never', { scope: 'global' })).toBeUndefined();
    expect(await memory.exists('never', { scope: 'global' })).toBe(false);
  });

  it('keeps each scope to itself', async () => {
    const memory = await backedByFake();
    await memory.set('who', 'session', { scope: 'session', scopeId: 's1' });
    await memory.set('who', 'actor', { scope: 'actor', scopeId: 'u1' });

    expect(await memory.get('who', { scope: 'session', scopeId: 's1' })).toBe('session');
    expect(await memory.get('who', { scope: 'actor', scopeId: 'u1' })).toBe('actor');
    expect(await memory.listKeys('session', { scopeId: 's1' })).toEqual(['who']);
  });

  it('deletes, and deleting what is absent is not an error', async () => {
    const memory = await backedByFake();
    await memory.set('temp', 1, { scope: 'session', scopeId: 's1' });
    await memory.delete('temp', { scope: 'session', scopeId: 's1' });

    expect(await memory.get('temp', { scope: 'session', scopeId: 's1' })).toBeUndefined();
    await expect(memory.delete('temp', { scope: 'session', scopeId: 's1' })).resolves.toBeUndefined();
  });

  it('lists every page, not the first', async () => {
    const memory = await backedByFake();
    const many = 250; // more than one 200-record page
    for (let i = 0; i < many; i++) {
      await memory.set(`k${String(i).padStart(3, '0')}`, i, { scope: 'global' });
    }
    expect((await memory.listKeys('global')).length).toBe(many);
  });

  it('ranks a similarity search, strongest first', async () => {
    const memory = await backedByFake();
    const where = { scope: 'session' as const, scopeId: 's1' };
    await memory.setVector('same', [1, 0, 0], undefined, where);
    await memory.setVector('near', [0.9, 0.4, 0], undefined, where);
    await memory.setVector('across', [0, 1, 0], undefined, where);

    const hits = await memory.searchVector([1, 0, 0], where);
    expect(hits.map((hit) => hit.key)).toEqual(['same', 'near', 'across']);
    expect(hits[0].score).toBeGreaterThan(0.999);
    expect(hits[0].scopeId).toBe('s1');
  });

  it('honours a metadata filter', async () => {
    const memory = await backedByFake();
    const where = { scope: 'session' as const, scopeId: 's1' };
    await memory.setVector('note', [1, 0], { kind: 'note' }, where);
    await memory.setVector('task', [1, 0], { kind: 'task' }, where);

    const hits = await memory.searchVector([1, 0], { ...where, filters: { kind: 'task' } });
    expect(hits.map((hit) => hit.key)).toEqual(['task']);
  });

  it('skips a vector of another width rather than scoring it', async () => {
    const memory = await backedByFake();
    const where = { scope: 'session' as const, scopeId: 's1' };
    await memory.setVector('wide', [1, 0, 0, 0], undefined, where);
    await memory.setVector('right', [1, 0], undefined, where);

    const hits = await memory.searchVector([1, 0], where);
    expect(hits.map((hit) => hit.key)).toEqual(['right']);
  });

  it('refuses a search with no query vector', async () => {
    const memory = await backedByFake();
    await expect(memory.searchVector([], { scope: 'session', scopeId: 's1' })).rejects.toThrow(
      /query vector/
    );
  });

  it('lets a value and an embedding share a key without either erasing the other', async () => {
    const memory = await backedByFake();
    const where = { scope: 'session' as const, scopeId: 's1' };

    await memory.setVector('doc', [1, 0], { kind: 'note' }, where);
    await memory.set('doc', 'the text', where);
    expect((await memory.searchVector([1, 0], where)).length).toBe(1);

    await memory.deleteVector('doc', where);
    expect((await memory.searchVector([1, 0], where)).length).toBe(0);
    expect(await memory.get('doc', where)).toBe('the text');
  });

  it('names a path Base does not serve, rather than failing to parse it', async () => {
    const memory = await backedByFake('wrong_collection');
    await expect(memory.get('anything', { scope: 'session', scopeId: 's1' })).rejects.toThrow(/HTML/);
  });

  it('quotes a scope id so a quote in one cannot end the clause', async () => {
    for (const odd of ["tenant' && scope='global", 'ends-with-backslash\\', "both\\' together"]) {
      const memory = await backedByFake();
      await memory.set('k', 'mine', { scope: 'actor', scopeId: odd });
      await memory.set('k', 'not mine', { scope: 'global' });

      expect(await memory.get('k', { scope: 'actor', scopeId: odd })).toBe('mine');
      await fake!.close();
    }
  });
});
