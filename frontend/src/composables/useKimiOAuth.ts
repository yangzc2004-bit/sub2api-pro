import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { KimiTokenInfo } from '@/api/admin/kimi'
import { extractApiErrorMessage } from '@/utils/apiError'

export function useKimiOAuth() {
  const appStore = useAppStore()
  const { t } = useI18n()

  const authUrl = ref('')
  const sessionId = ref('')
  const userCode = ref('')
  const interval = ref(5)
  const loading = ref(false)
  const error = ref('')

  const resetState = () => {
    authUrl.value = ''
    sessionId.value = ''
    userCode.value = ''
    interval.value = 5
    loading.value = false
    error.value = ''
  }

  const generateAuthUrl = async (proxyId?: number | null): Promise<boolean> => {
    loading.value = true
    error.value = ''
    authUrl.value = ''
    sessionId.value = ''
    userCode.value = ''
    try {
      const payload: { proxy_id?: number } = {}
      if (proxyId) payload.proxy_id = proxyId
      const response = await adminAPI.kimi.generateAuthUrl(payload)
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      userCode.value = response.user_code || ''
      interval.value = response.interval || 5
      return true
    } catch (err: any) {
      error.value = extractApiErrorMessage(err, 'Failed to generate Kimi auth URL')
      appStore.showError(error.value)
      return false
    } finally {
      loading.value = false
    }
  }

  const exchangeDeviceCode = async (
    currentSessionId: string,
    proxyId?: number | null
  ): Promise<KimiTokenInfo | null> => {
    if (!currentSessionId) {
      error.value = 'Missing session ID'
      return null
    }
    loading.value = true
    error.value = ''
    try {
      const payload: { session_id: string; proxy_id?: number } = { session_id: currentSessionId }
      if (proxyId) payload.proxy_id = proxyId
      return await adminAPI.kimi.exchangeCode(payload)
    } catch (err: any) {
      error.value = extractApiErrorMessage(err, 'Kimi authorization is not complete yet')
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  const validateRefreshToken = async (
    refreshToken: string,
    proxyId?: number | null
  ): Promise<KimiTokenInfo | null> => {
    if (!refreshToken.trim()) {
      error.value = 'Missing refresh token'
      return null
    }
    loading.value = true
    error.value = ''
    try {
      return await adminAPI.kimi.refreshToken(refreshToken.trim(), proxyId)
    } catch (err: any) {
      error.value = extractApiErrorMessage(err, t('admin.accounts.oauth.authFailed'))
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  const buildCredentials = (tokenInfo: KimiTokenInfo): Record<string, unknown> => {
    const creds: Record<string, unknown> = {
      access_token: tokenInfo.access_token,
      expires_at: tokenInfo.expires_at
    }
    if (tokenInfo.refresh_token) creds.refresh_token = tokenInfo.refresh_token
    if (tokenInfo.token_type) creds.token_type = tokenInfo.token_type
    if (tokenInfo.scope) creds.scope = tokenInfo.scope
    if (tokenInfo.client_id) creds.client_id = tokenInfo.client_id
    return creds
  }

  return {
    authUrl,
    sessionId,
    userCode,
    interval,
    loading,
    error,
    resetState,
    generateAuthUrl,
    exchangeDeviceCode,
    validateRefreshToken,
    buildCredentials
  }
}
