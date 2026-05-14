import { apiClient } from '../client'

export interface KimiAuthURLResponse {
  auth_url: string
  session_id: string
  user_code?: string
  device_code?: string
  expires_in?: number
  interval?: number
}

export interface KimiTokenInfo {
  access_token?: string
  refresh_token?: string
  token_type?: string
  expires_in?: number
  expires_at?: number
  scope?: string
  client_id?: string
  [key: string]: unknown
}

export async function generateAuthUrl(payload: { proxy_id?: number } = {}): Promise<KimiAuthURLResponse> {
  const { data } = await apiClient.post<KimiAuthURLResponse>('/admin/kimi/oauth/auth-url', payload)
  return data
}

export async function exchangeCode(payload: { session_id: string; proxy_id?: number }): Promise<KimiTokenInfo> {
  const { data } = await apiClient.post<KimiTokenInfo>('/admin/kimi/oauth/exchange-code', payload)
  return data
}

export async function refreshToken(refreshToken: string, proxyId?: number | null): Promise<KimiTokenInfo> {
  const payload: Record<string, unknown> = { refresh_token: refreshToken }
  if (proxyId) payload.proxy_id = proxyId
  const { data } = await apiClient.post<KimiTokenInfo>('/admin/kimi/oauth/refresh-token', payload)
  return data
}

export default { generateAuthUrl, exchangeCode, refreshToken }
