import type express from 'express';
import { ExecutionContext } from './ExecutionContext.js';
import type { Agent } from '../agent/Agent.js';
import type { MemoryInterface } from '../memory/MemoryInterface.js';
import type { WorkflowReporter } from '../workflow/WorkflowReporter.js';
import type { DiscoveryOptions } from '../types/agent.js';
import type { DidInterface } from '../did/DidInterface.js';

export class SkillContext<TInput = any> {
  readonly input: TInput;
  readonly executionId: string;
  readonly sessionId?: string;
  readonly workflowId?: string;
  readonly callerDid?: string;
  readonly agentNodeDid?: string;
  readonly req: express.Request;
  readonly res: express.Response;
  readonly agent: Agent;
  readonly memory: MemoryInterface;
  readonly workflow: WorkflowReporter;
  readonly did: DidInterface;

  constructor(params: {
    input: TInput;
    executionId: string;
    sessionId?: string;
    workflowId?: string;
    callerDid?: string;
    agentNodeDid?: string;
    req: express.Request;
    res: express.Response;
    agent: Agent;
    memory: MemoryInterface;
    workflow: WorkflowReporter;
    did: DidInterface;
  }) {
    this.input = params.input;
    this.executionId = params.executionId;
    this.sessionId = params.sessionId;
    this.workflowId = params.workflowId;
    this.callerDid = params.callerDid;
    this.agentNodeDid = params.agentNodeDid;
    this.req = params.req;
    this.res = params.res;
    this.agent = params.agent;
    this.memory = params.memory;
    this.workflow = params.workflow;
    this.did = params.did;
  }

  discover(options?: DiscoveryOptions) {
    return this.agent.discover(options);
  }
}

export function getCurrentSkillContext<TInput = any>(): SkillContext<TInput> | undefined {
  const execution = ExecutionContext.getCurrent();
  if (!execution) return undefined;
  const { metadata, input, agent, req, res } = execution;
  return new SkillContext<TInput>({
    input,
    executionId: metadata.executionId,
    sessionId: metadata.sessionId,
    workflowId: metadata.workflowId,
    callerDid: metadata.callerDid,
    agentNodeDid: metadata.agentNodeDid,
    req,
    res,
    agent,
    memory: agent.getMemoryInterface(metadata),
    workflow: agent.getWorkflowReporter(metadata),
    did: agent.getDidInterface(metadata, input)
  });
}
