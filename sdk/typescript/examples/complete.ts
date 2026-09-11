/**
 * The shortest thing this SDK does: one prompt, one answer.
 *
 * Run it with a key in the environment:
 *
 *   HANZO_API_KEY=… npx tsx examples/complete.ts
 *
 * `baseUrl` is set rather than left to the provider default. `provider` is the
 * wire dialect — our gateway speaks the OpenAI one — and `baseUrl` is the
 * address. An example that set only the provider would talk to somebody else's
 * endpoint with our key, which is the one mistake worth designing out of a
 * first example.
 */
import { AIClient } from '../src/ai/AIClient.js';

async function main(): Promise<void> {
  const client = new AIClient({
    provider: 'openai',
    baseUrl: 'https://api.hanzo.ai/v1',
    apiKey: process.env.HANZO_API_KEY,
    model: 'zen-3',
    temperature: 0,
  });

  const answer = await client.generate('Name the four suits of a standard deck of cards.');
  console.log(answer);
}

main().catch((error: unknown) => {
  // The message, not the object: an unhandled rejection prints a stack over a
  // string nobody reads, and a key can end up in a dumped request object.
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
});
