/**
 * The shortest thing this SDK does: one prompt, one answer.
 *
 *   HANZO_API_KEY=… npx tsx examples/complete.ts
 *
 * No provider and no address: `hanzo` is the default and it resolves to
 * api.hanzo.ai. Naming another provider selects it, and the three that speak
 * the OpenAI dialect bring their own address with them.
 */
import { AIClient } from '../src/ai/AIClient.js';

async function main(): Promise<void> {
  const client = new AIClient({ apiKey: process.env.HANZO_API_KEY });

  const answer = await client.generate('Name the four suits of a standard deck of cards.');
  console.log(answer);
}

main().catch((error: unknown) => {
  // The message, not the object: an unhandled rejection prints a stack over a
  // string nobody reads, and a dumped request object can carry the key.
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
});
