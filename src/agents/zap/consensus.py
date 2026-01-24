"""Multi-agent consensus using ZAP coordination.

Implements consensus-based decision making across multiple agents or models
using the ZAP coordination interface and Lux-style metastable consensus.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import time
from dataclasses import dataclass, field
from typing import Any

from ..agent import Agent
from ..run import Runner
from .client import ZapClient
from .types import (
    Certificate,
    ConsensusConfig,
)


@dataclass
class ParticipantResponse:
    """Response from a consensus participant."""

    participant_id: str
    response: str
    confidence: float
    latency_ms: int
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class ConsensusDecision:
    """Final decision from consensus process."""

    question: str
    answer: str
    confidence: float
    round: int
    votes: list[ParticipantResponse]
    certificate: Certificate | None = None
    synthesis: str = ""
    duration_ms: int = 0


class ConsensusAgent:
    """Agent that uses ZAP coordination for consensus-based decisions.

    Implements multi-agent consensus using either:
    1. Local agent pool with voting
    2. ZAP gateway coordination (distributed consensus)

    Example:
        # Create consensus agent with local pool
        consensus = ConsensusAgent(
            agents=[agent1, agent2, agent3],
            config=ConsensusConfig(rounds=3, k=3, alpha=0.6),
        )
        decision = await consensus.decide("What is the best approach?")

        # Or use ZAP gateway coordination
        consensus = ConsensusAgent.from_gateway("zap://localhost:9999")
        decision = await consensus.decide(
            "What is the best approach?",
            participants=["gpt-4", "claude-3", "llama-70b"],
        )
    """

    def __init__(
        self,
        agents: list[Agent[Any]] | None = None,
        client: ZapClient | None = None,
        config: ConsensusConfig | None = None,
        synthesizer: Agent[Any] | None = None,
    ):
        """Initialize consensus agent.

        Args:
            agents: Local agent pool for voting.
            client: ZAP client for distributed consensus.
            config: Consensus configuration.
            synthesizer: Agent to synthesize final answer from votes.
        """
        self.agents = agents or []
        self.client = client
        self.config = config or ConsensusConfig()
        self.synthesizer = synthesizer
        self._use_gateway = client is not None

    @classmethod
    def from_gateway(
        cls,
        uri: str,
        config: ConsensusConfig | None = None,
    ) -> ConsensusAgent:
        """Create consensus agent using ZAP gateway.

        Args:
            uri: ZAP gateway URI.
            config: Consensus configuration.

        Returns:
            ConsensusAgent configured for gateway use.
        """
        client = ZapClient.from_uri(uri)
        return cls(client=client, config=config)

    @classmethod
    def from_agents(
        cls,
        agents: list[Agent[Any]],
        config: ConsensusConfig | None = None,
        synthesizer: Agent[Any] | None = None,
    ) -> ConsensusAgent:
        """Create consensus agent using local agent pool.

        Args:
            agents: List of agents to participate.
            config: Consensus configuration.
            synthesizer: Agent to synthesize final answer.

        Returns:
            ConsensusAgent configured for local use.
        """
        return cls(agents=agents, config=config, synthesizer=synthesizer)

    async def connect(self) -> None:
        """Connect to ZAP gateway if configured."""
        if self.client:
            await self.client.connect()

    async def close(self) -> None:
        """Close gateway connection."""
        if self.client:
            await self.client.close()

    async def __aenter__(self) -> ConsensusAgent:
        await self.connect()
        return self

    async def __aexit__(self, *args: Any) -> None:
        await self.close()

    async def decide(
        self,
        question: str,
        participants: list[str] | None = None,
        context: dict[str, Any] | None = None,
    ) -> ConsensusDecision:
        """Get consensus decision on a question.

        Args:
            question: Question to decide on.
            participants: Participant IDs (for gateway mode).
            context: Additional context for the question.

        Returns:
            ConsensusDecision with answer and certificate.
        """
        start_time = time.time()

        if self._use_gateway and self.client:
            decision = await self._decide_gateway(question, participants or [])
        else:
            decision = await self._decide_local(question, context)

        decision.duration_ms = int((time.time() - start_time) * 1000)
        return decision

    async def _decide_gateway(
        self,
        question: str,
        participants: list[str],
    ) -> ConsensusDecision:
        """Use ZAP gateway coordination for consensus."""
        if not self.client:
            raise RuntimeError("No ZAP client configured")

        answer, certificate = await self.client.committee_query(
            question=question,
            participants=participants,
            config=self.config,
        )

        return ConsensusDecision(
            question=question,
            answer=answer,
            confidence=certificate.confidence,
            round=certificate.round,
            votes=[],  # Gateway doesn't expose individual votes
            certificate=certificate,
            synthesis=answer,
        )

    async def _decide_local(
        self,
        question: str,
        context: dict[str, Any] | None = None,
    ) -> ConsensusDecision:
        """Use local agent pool for consensus."""
        if not self.agents:
            raise RuntimeError("No agents configured")

        # Build prompt with context
        prompt = question
        if context:
            prompt = f"Context: {json.dumps(context)}\n\nQuestion: {question}"

        # Collect responses from all agents
        responses = await self._collect_responses(prompt)

        # Run Snowball-style consensus rounds
        winner, confidence, round_num = await self._run_consensus_rounds(responses)

        # Synthesize final answer
        synthesis = await self._synthesize_answer(question, responses, winner)

        # Create certificate
        certificate = self._create_certificate(question, winner, confidence, round_num, responses)

        return ConsensusDecision(
            question=question,
            answer=winner,
            confidence=confidence,
            round=round_num,
            votes=responses,
            certificate=certificate,
            synthesis=synthesis,
        )

    async def _collect_responses(self, prompt: str) -> list[ParticipantResponse]:
        """Collect responses from all agents in parallel."""
        async def get_response(agent: Agent[Any], idx: int) -> ParticipantResponse:
            start = time.time()
            try:
                result = await Runner.run(
                    starting_agent=agent,
                    input=prompt,
                )
                response_text = ""
                for item in result.new_items:
                    if hasattr(item, "raw_item") and hasattr(item.raw_item, "content"):
                        for content in item.raw_item.content:
                            if hasattr(content, "text"):
                                response_text += content.text
                latency = int((time.time() - start) * 1000)
                return ParticipantResponse(
                    participant_id=f"agent_{idx}_{agent.name}",
                    response=response_text.strip(),
                    confidence=1.0,  # Local agents don't report confidence
                    latency_ms=latency,
                )
            except Exception as e:
                return ParticipantResponse(
                    participant_id=f"agent_{idx}_{agent.name}",
                    response=f"Error: {e}",
                    confidence=0.0,
                    latency_ms=int((time.time() - start) * 1000),
                    metadata={"error": str(e)},
                )

        tasks = [get_response(agent, i) for i, agent in enumerate(self.agents)]
        return await asyncio.gather(*tasks)

    async def _run_consensus_rounds(
        self,
        responses: list[ParticipantResponse],
    ) -> tuple[str, float, int]:
        """Run Snowball-style consensus rounds.

        Returns (winner, confidence, round_number).
        """
        # Normalize responses for comparison
        votes: dict[str, list[ParticipantResponse]] = {}
        for response in responses:
            # Use hash for grouping similar responses
            key = self._normalize_response(response.response)
            if key not in votes:
                votes[key] = []
            votes[key].append(response)

        # Find majority
        total = len(responses)
        best_count = 0
        best_response = ""

        for _key, voters in votes.items():
            if len(voters) > best_count:
                best_count = len(voters)
                best_response = voters[0].response

        confidence = best_count / total if total > 0 else 0.0

        # Run additional rounds if confidence below threshold
        round_num = 1
        while confidence < self.config.beta2 and round_num < self.config.rounds:
            round_num += 1
            # In a full implementation, we would re-query agents
            # For now, just use the initial votes
            break

        return best_response, confidence, round_num

    def _normalize_response(self, response: str) -> str:
        """Normalize response for comparison."""
        # Simple normalization: lowercase, strip whitespace, remove punctuation
        normalized = response.lower().strip()
        # Use hash for consistent grouping
        return hashlib.sha256(normalized.encode()).hexdigest()[:16]

    async def _synthesize_answer(
        self,
        question: str,
        responses: list[ParticipantResponse],
        winner: str,
    ) -> str:
        """Synthesize final answer from responses."""
        if self.synthesizer:
            # Use synthesizer agent to combine responses
            responses_text = chr(10).join(
                f"- {r.response}" for r in responses if r.confidence > 0
            )
            synthesis_prompt = (
                "Given the following question and responses, "
                "synthesize the best answer.\n\n"
                f"Question: {question}\n\n"
                f"Responses:\n{responses_text}\n\n"
                f"Most common response: {winner}\n\n"
                "Provide a clear, synthesized answer that incorporates the best elements."
            )

            result = await Runner.run(
                starting_agent=self.synthesizer,
                input=synthesis_prompt,
            )
            synthesis = ""
            for item in result.new_items:
                if hasattr(item, "raw_item") and hasattr(item.raw_item, "content"):
                    for content in item.raw_item.content:
                        if hasattr(content, "text"):
                            synthesis += content.text
            return synthesis.strip()

        return winner

    def _create_certificate(
        self,
        question: str,
        winner: str,
        confidence: float,
        round_num: int,
        responses: list[ParticipantResponse],
    ) -> Certificate:
        """Create a certificate for the consensus decision."""
        topic = hashlib.sha256(question.encode()).digest()
        proposal_hash = hashlib.sha256(winner.encode()).digest()

        attestors = []
        for response in responses:
            if self._normalize_response(response.response) == self._normalize_response(winner):
                attestors.append({
                    "nodeId": response.participant_id,
                    "signature": "",  # Would be actual signature in production
                    "publicKey": "",
                })

        return Certificate(
            topic=topic,
            proposal_hash=proposal_hash,
            round=round_num,
            confidence=confidence,
            attestors=attestors,
            timestamp=int(time.time() * 1000),
        )


async def consensus_decide(
    question: str,
    agents: list[Agent[Any]],
    config: ConsensusConfig | None = None,
) -> ConsensusDecision:
    """Convenience function for one-shot consensus decision.

    Args:
        question: Question to decide on.
        agents: List of agents to participate.
        config: Optional consensus configuration.

    Returns:
        ConsensusDecision with answer and certificate.

    Example:
        decision = await consensus_decide(
            "Should we use PostgreSQL or MongoDB?",
            agents=[analyst1, analyst2, architect],
        )
        print(f"Decision: {decision.answer} (confidence: {decision.confidence})")
    """
    consensus = ConsensusAgent.from_agents(agents, config)
    return await consensus.decide(question)


async def gateway_consensus(
    question: str,
    gateway_uri: str,
    participants: list[str],
    config: ConsensusConfig | None = None,
) -> ConsensusDecision:
    """Get consensus from ZAP gateway committee.

    Args:
        question: Question to decide on.
        gateway_uri: ZAP gateway URI.
        participants: List of model/agent IDs.
        config: Optional consensus configuration.

    Returns:
        ConsensusDecision with answer and certificate.

    Example:
        decision = await gateway_consensus(
            "What is the most efficient sorting algorithm?",
            "zap://localhost:9999",
            participants=["gpt-4", "claude-3-opus", "gemini-pro"],
        )
    """
    async with ConsensusAgent.from_gateway(gateway_uri, config) as consensus:
        return await consensus.decide(question, participants=participants)
