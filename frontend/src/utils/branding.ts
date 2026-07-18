import { sanitizeUrl } from '@/utils/url'

const DEFAULT_SITE_NAME = 'Sub2API'
const DISPLAY_SITE_NAME = 'Sub2API-Pro'

export function formatSiteName(siteName?: string | null): string {
  const normalized = siteName?.trim()
  if (!normalized || normalized === DEFAULT_SITE_NAME) {
    return DISPLAY_SITE_NAME
  }
  return normalized
}

export function updateFavicon(logoUrl: string): void {
  const sanitizedLogoUrl = sanitizeUrl(logoUrl, {
    allowRelative: true,
    allowDataUrl: true,
  })
  if (!sanitizedLogoUrl) {
    return
  }

  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }

  link.type = sanitizedLogoUrl.endsWith('.svg') ? 'image/svg+xml' : 'image/x-icon'
  link.href = sanitizedLogoUrl
}
