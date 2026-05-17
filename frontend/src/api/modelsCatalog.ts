import { apiClient } from './client'

export interface ModelCatalogPricing {
  unit: 'per_1m_tokens'
  currency: string
  display_symbol: string
  input: number | null
  output: number | null
  cache_write: number | null
  cache_read: number | null
}

export interface ModelCatalogModel {
  id: string
  name: string
  description: string
  tags: string[]
  context_tokens: number
  output_tokens: number
  pricing: ModelCatalogPricing
  pricing_source: 'official' | 'override' | string
}

export interface ModelCatalogProvider {
  id: string
  name: string
  display_name: string
  logo_key: string
  models: ModelCatalogModel[]
}

export async function getCatalog(options?: { signal?: AbortSignal }): Promise<ModelCatalogProvider[]> {
  const { data } = await apiClient.get<ModelCatalogProvider[]>('/models/catalog', {
    signal: options?.signal
  })
  return data
}

export interface ModelCatalogPricingPayload {
  input: number | null
  output: number | null
  cache_write: number | null
  cache_read: number | null
}

export async function getAdminCatalog(options?: { signal?: AbortSignal }): Promise<ModelCatalogProvider[]> {
  const { data } = await apiClient.get<ModelCatalogProvider[]>('/admin/models/catalog', {
    signal: options?.signal
  })
  return data
}

export async function updateAdminPricing(modelId: string, payload: ModelCatalogPricingPayload): Promise<ModelCatalogModel> {
  const encodedModelId = encodeURIComponent(modelId).replace(/\./g, '%2E')
  const { data } = await apiClient.put<ModelCatalogModel>(
    `/admin/models/catalog/${encodedModelId}/pricing`,
    payload,
  )
  return data
}

export async function resetAdminPricing(): Promise<void> {
  await apiClient.post('/admin/models/catalog/reset')
}

export const modelsCatalogAPI = { getCatalog, getAdminCatalog, updateAdminPricing, resetAdminPricing }

export default modelsCatalogAPI
