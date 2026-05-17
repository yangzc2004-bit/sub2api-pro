const DEFAULT_SITE_NAME = 'Sub2API'
const DISPLAY_SITE_NAME = 'Sub2API-Pro'

export function formatSiteName(siteName?: string | null): string {
  const normalized = siteName?.trim()
  if (!normalized || normalized === DEFAULT_SITE_NAME) {
    return DISPLAY_SITE_NAME
  }
  return normalized
}
