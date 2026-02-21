/**
 * White-label branding configuration.
 *
 * Override via environment variables:
 *   VITE_BRAND_NAME       - Product name (default: "Hanzo Bot")
 *   VITE_BRAND_ENTITY     - What a single entity is called (default: "Bot")
 *   VITE_BRAND_ENTITY_PL  - Plural form (default: "Bots")
 *   VITE_BRAND_LOGO_URL   - Path to logo SVG (default: "/hanzo.svg")
 *   VITE_BRAND_ACCENT     - Primary accent color (default: inherited from theme)
 */

export const brand = {
  /** Product name shown in sidebar header */
  name: import.meta.env.VITE_BRAND_NAME || 'Hanzo Bot',

  /** Singular entity label: "Bot", "Agent", etc. */
  entity: import.meta.env.VITE_BRAND_ENTITY || 'Bot',

  /** Plural entity label */
  entityPlural: import.meta.env.VITE_BRAND_ENTITY_PL || 'Bots',

  /** Path to brand logo SVG */
  logoUrl: import.meta.env.VITE_BRAND_LOGO_URL || '/hanzo.svg',

  /** Version string */
  version: import.meta.env.VITE_BRAND_VERSION || 'hanzo-bot',
} as const;

/** Helper: "My Bots", "Add Bot", etc. */
export const label = {
  my: `My ${brand.entityPlural}`,
  add: `Add ${brand.entity}`,
  new: `New ${brand.entity}`,
  create: `Create ${brand.entity}`,
  start: `Start ${brand.entity}`,
  stop: `Stop ${brand.entity}`,
  remove: `Remove ${brand.entity}`,
  configure: `Configure ${brand.entity}`,
  search: `Search ${brand.entityPlural.toLowerCase()}...`,
  noneFound: `No ${brand.entityPlural.toLowerCase()}`,
  noMatch: `No ${brand.entityPlural.toLowerCase()} match your search.`,
  createPrompt: `Create a ${brand.entity.toLowerCase()} to get started.`,
  customEntity: `Custom ${brand.entity}`,
} as const;
