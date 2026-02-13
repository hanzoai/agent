import { AsyncLocalStorage } from 'node:async_hooks';
import type express from 'express';
import type { Agent } from '../agent/Agent.js';

export interface ExecutionMetadata {
  executionId: string;
  runId?: string;
  sessionId?: string;
  actorId?: string;
  workflowId?: string;
  parentExecutionId?: string;
  callerDid?: string;
  targetDid?: string;
  agentNodeDid?: string;
}

const store = new AsyncLocalStorage<ExecutionContext>();

export class ExecutionContext {
  readonly input: any;
  readonly metadata: ExecutionMetadata;
  readonly req: express.Request;
  readonly res: express.Response;
  readonly agent: Agent;

  constructor(params: {
    input: any;
    metadata: ExecutionMetadata;
    req: express.Request;
    res: express.Response;
    agent: Agent;
  }) {
    this.input = params.input;
    this.metadata = params.metadata;
    this.req = params.req;
    this.res = params.res;
    this.agent = params.agent;
  }

  static run<T>(ctx: ExecutionContext, fn: () => T): T {
    return store.run(ctx, fn);
  }

  static getCurrent(): ExecutionContext | undefined {
    return store.getStore();
  }
}
