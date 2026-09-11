/**
 * The same call, returning a typed object rather than prose.
 *
 * `generate` is overloaded: hand it a schema and it resolves to that shape
 * instead of a string, so the result needs no parsing and no cast.
 *
 *   HANZO_API_KEY=… npx tsx examples/structured.ts
 */
import * as z from 'zod';
import { AIClient } from '../src/ai/AIClient.js';

const Card = z.object({
  suit: z.enum(['hearts', 'diamonds', 'clubs', 'spades']),
  rank: z.string(),
});

async function main(): Promise<void> {
  const client = new AIClient({
    provider: 'openai',
    baseUrl: 'https://api.hanzo.ai/v1',
    apiKey: process.env.HANZO_API_KEY,
  });

  // The schema is the second argument's `schema` field, and the return type
  // follows from it — `card.suit` is the enum, checked at compile time.
  const card = await client.generate('Pick one playing card.', { schema: Card });
  console.log(`${card.rank} of ${card.suit}`);
}

main().catch((error: unknown) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
});
