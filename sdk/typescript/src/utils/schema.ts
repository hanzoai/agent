import { toJSONSchema, type ZodType } from 'zod';

/**
 * Check if a value is a Zod schema by looking for Zod's internal structure.
 */
function isZodSchema(value: unknown): value is ZodType {
  if (!value || typeof value !== 'object') return false;
  const obj = value as Record<string, unknown>;
  // Zod schemas have _def with typeName, or ~standard with vendor: 'zod'
  return (
    ('_def' in obj && typeof obj._def === 'object') ||
    ('~standard' in obj && (obj['~standard'] as Record<string, unknown>)?.vendor === 'zod')
  );
}

/**
 * Convert a schema to JSON Schema format.
 * If the input is a Zod schema, converts it with zod's own converter.
 * If the input is already a plain object (assumed to be JSON Schema), returns it as-is.
 * If the input is undefined/null, returns an empty object.
 */
export function toJsonSchema(schema: unknown): Record<string, unknown> {
  if (schema === undefined || schema === null) {
    return {};
  }

  if (isZodSchema(schema)) {
    // zod's own converter, which v4 ships. zod-to-json-schema reads v3's
    // internals and cannot type a v4 schema at all — this package declares
    // zod ^4, so the library it was calling was one major version behind the
    // schemas handed to it.
    //
    // openApi-3.0 for tool calling, as before, and it emits no `$schema` — so
    // the property this used to strip afterwards is one that never arrives.
    return toJSONSchema(schema as ZodType, {
      target: 'openApi-3.0',
    }) as Record<string, unknown>;
  }

  // Assume it's already a JSON Schema or plain object
  if (typeof schema === 'object') {
    return schema as Record<string, unknown>;
  }

  return {};
}
