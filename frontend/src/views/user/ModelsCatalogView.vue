<template>
  <AppLayout>
    <div class="models-catalog-page">
      <div class="catalog-heading">
        <div>
          <p class="catalog-kicker">{{ t('modelsCatalog.kicker') }}</p>
          <h1>
            <span>{{ t('modelsCatalog.title') }}</span>
            <span class="catalog-price-note">({{ t('modelsCatalog.priceMultiplierNote') }})</span>
          </h1>
          <p>{{ t('modelsCatalog.description') }}</p>
        </div>
        <button class="btn btn-secondary" type="button" :disabled="loading" @click="loadCatalog">
          <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          <span>{{ t('common.refresh', 'Refresh') }}</span>
        </button>
      </div>

      <div class="catalog-layout">
        <aside class="catalog-filters glass-panel" aria-label="Model filters">
          <div class="filter-block">
            <div class="filter-title-row">
              <div>
                <h2>{{ t('modelsCatalog.filters.title') }}</h2>
                <p>{{ t('modelsCatalog.filters.description') }}</p>
              </div>
              <button class="icon-button" type="button" :title="t('modelsCatalog.reset')" @click="resetFilters">
                <Icon name="refresh" size="sm" />
              </button>
            </div>

            <div class="relative">
              <Icon name="search" size="md" class="search-icon" />
              <input
                v-model="searchQuery"
                type="text"
                class="input catalog-search"
                :placeholder="t('modelsCatalog.searchPlaceholder')"
              />
            </div>
          </div>

          <div class="filter-block">
            <button class="filter-section-heading" type="button" @click="providersExpanded = !providersExpanded">
              <span>{{ t('modelsCatalog.filters.providers') }}</span>
              <Icon name="chevronDown" size="sm" :class="providersExpanded ? 'rotate-180' : ''" />
            </button>
            <div v-if="providersExpanded" class="chip-grid">
              <button
                type="button"
                class="filter-chip"
                :class="{ active: selectedProviderId === 'all' }"
                @click="selectedProviderId = 'all'"
              >
                <span>{{ t('modelsCatalog.allProviders') }}</span>
                <strong>{{ totalModels }}</strong>
              </button>
              <button
                v-for="provider in providers"
                :key="provider.id"
                type="button"
                class="filter-chip"
                :class="{ active: selectedProviderId === provider.id }"
                @click="selectedProviderId = provider.id"
              >
                <PlatformIcon :platform="provider.logo_key" size="xs" />
                <span>{{ provider.display_name }}</span>
                <strong>{{ provider.models.length }}</strong>
              </button>
            </div>
          </div>

          <div class="filter-block">
            <button class="filter-section-heading" type="button" @click="tagsExpanded = !tagsExpanded">
              <span>{{ t('modelsCatalog.filters.tags') }}</span>
              <Icon name="chevronDown" size="sm" :class="tagsExpanded ? 'rotate-180' : ''" />
            </button>
            <div v-if="tagsExpanded" class="chip-grid">
              <button
                v-for="tag in tagOptions"
                :key="tag.name"
                type="button"
                class="filter-chip"
                :class="{ active: selectedTags.includes(tag.name) }"
                @click="toggleTag(tag.name)"
              >
                <span>{{ formatTag(tag.name) }}</span>
                <strong>{{ tag.count }}</strong>
              </button>
            </div>
          </div>

          <div class="filter-block">
            <button class="filter-section-heading" type="button" @click="pricingExpanded = !pricingExpanded">
              <span>{{ t('modelsCatalog.filters.priceField') }}</span>
              <Icon name="chevronDown" size="sm" :class="pricingExpanded ? 'rotate-180' : ''" />
            </button>
            <div v-if="pricingExpanded" class="chip-grid">
              <button
                v-for="field in priceFields"
                :key="field"
                type="button"
                class="filter-chip"
                :class="{ active: selectedPriceField === field }"
                @click="selectedPriceField = field"
              >
                <span>{{ t(`modelsCatalog.priceFields.${field}`) }}</span>
              </button>
            </div>
          </div>
        </aside>

        <main class="catalog-main">
          <section class="catalog-toolbar glass-panel">
            <div class="result-count">
              <strong>{{ filteredModels.length }}</strong>
              <span>{{ t('modelsCatalog.countSuffix', { total: totalModels }) }}</span>
            </div>

            <div class="toolbar-controls">
              <div class="segmented" role="group" :aria-label="t('modelsCatalog.priceMode.label')">
                <button
                  v-for="mode in priceModes"
                  :key="mode"
                  type="button"
                  :class="{ active: priceMode === mode }"
                  @click="priceMode = mode"
                >
                  {{ t(`modelsCatalog.priceMode.${mode}`) }}
                </button>
              </div>
              <div class="segmented" role="group" :aria-label="t('modelsCatalog.unit.label')">
                <button
                  v-for="option in unitOptions"
                  :key="option"
                  type="button"
                  :class="{ active: unitMode === option }"
                  @click="unitMode = option"
                >
                  {{ t(`modelsCatalog.unit.${option}`) }}
                </button>
              </div>
              <label class="sort-select">
                <Icon name="sort" size="sm" />
                <select v-model="sortBy">
                  <option value="name">{{ t('modelsCatalog.sort.name') }}</option>
                  <option value="input">{{ t('modelsCatalog.sort.input') }}</option>
                  <option value="output">{{ t('modelsCatalog.sort.output') }}</option>
                  <option value="context">{{ t('modelsCatalog.sort.context') }}</option>
                </select>
              </label>
            </div>
          </section>

          <section v-if="loading" class="models-grid" aria-busy="true">
            <article v-for="idx in 6" :key="idx" class="model-card skeleton-card">
              <div class="skeleton-line w-2/3"></div>
              <div class="skeleton-line w-full"></div>
              <div class="skeleton-line w-5/6"></div>
              <div class="skeleton-metrics"></div>
            </article>
          </section>

          <section v-else-if="errorMessage" class="empty-panel glass-panel error-panel">
            <Icon name="exclamationCircle" size="xl" />
            <h2>{{ t('modelsCatalog.error.title') }}</h2>
            <p>{{ errorMessage }}</p>
            <button class="btn btn-secondary" type="button" @click="loadCatalog">
              <Icon name="refresh" size="sm" />
              <span>{{ t('modelsCatalog.error.retry') }}</span>
            </button>
          </section>

          <section v-else-if="filteredModels.length" class="models-grid">
            <article v-for="item in filteredModels" :key="item.model.id" class="model-card glass-panel">
              <header class="model-card-header">
                <div class="model-mark">
                  <ModelIcon :model="item.model.id" size="22px" />
                </div>
                <div class="min-w-0">
                  <h2 :title="item.model.name">{{ item.model.name }}</h2>
                  <p>{{ item.provider.display_name }}</p>
                </div>
                <button
                  class="icon-button"
                  type="button"
                  :title="t('modelsCatalog.copyModel')"
                  @click="copyToClipboard(item.model.id, t('modelsCatalog.copied'))"
                >
                  <Icon name="copy" size="sm" />
                </button>
              </header>

              <p class="model-description">{{ item.model.description || t('modelsCatalog.noDescription') }}</p>

              <div class="tag-row">
                <span v-for="tag in item.model.tags" :key="tag" class="tag-pill">{{ formatTag(tag) }}</span>
              </div>

              <div class="pricing-grid" :class="{ cache: priceMode === 'cache' }">
                <div class="price-cell primary">
                  <span>{{ t('modelsCatalog.pricing.input') }}</span>
                  <strong>{{ formatPrice(item.model.pricing.input) }}</strong>
                </div>
                <div class="price-cell primary">
                  <span>{{ t('modelsCatalog.pricing.output') }}</span>
                  <strong>{{ formatPrice(item.model.pricing.output) }}</strong>
                </div>
                <div class="price-cell">
                  <span>{{ t('modelsCatalog.pricing.cacheWrite') }}</span>
                  <strong>{{ formatPrice(item.model.pricing.cache_write) }}</strong>
                </div>
                <div class="price-cell">
                  <span>{{ t('modelsCatalog.pricing.cacheRead') }}</span>
                  <strong>{{ formatPrice(item.model.pricing.cache_read) }}</strong>
                </div>
              </div>

              <footer class="model-meta-row">
                <span>{{ t('modelsCatalog.context') }} {{ formatTokenCount(item.model.context_tokens) }}</span>
                <span>{{ t('modelsCatalog.output') }} {{ formatTokenCount(item.model.output_tokens) }}</span>
                <span>{{ t('modelsCatalog.sourceOfficial') }}</span>
              </footer>
            </article>
          </section>

          <section v-else class="empty-panel glass-panel">
            <Icon name="search" size="xl" />
            <h2>{{ t('modelsCatalog.empty.title') }}</h2>
            <p>{{ t('modelsCatalog.empty.description') }}</p>
            <button class="btn btn-secondary" type="button" @click="resetFilters">
              {{ t('modelsCatalog.reset') }}
            </button>
          </section>
        </main>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import modelsCatalogAPI, { type ModelCatalogModel, type ModelCatalogProvider } from '@/api/modelsCatalog'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { useClipboard } from '@/composables/useClipboard'

type PriceField = 'all' | 'input' | 'output' | 'cache'
type PriceMode = 'standard' | 'cache'
type UnitMode = '1m' | '1k'
type SortBy = 'name' | 'input' | 'output' | 'context'

interface CatalogModelItem {
  provider: ModelCatalogProvider
  model: ModelCatalogModel
}

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const providers = ref<ModelCatalogProvider[]>([])
const loading = ref(false)
const errorMessage = ref('')
const searchQuery = ref('')
const selectedProviderId = ref('all')
const selectedTags = ref<string[]>([])
const selectedPriceField = ref<PriceField>('all')
const priceMode = ref<PriceMode>('standard')
const unitMode = ref<UnitMode>('1m')
const sortBy = ref<SortBy>('name')
const providersExpanded = ref(true)
const tagsExpanded = ref(true)
const pricingExpanded = ref(true)

const priceFields: PriceField[] = ['all', 'input', 'output', 'cache']
const priceModes: PriceMode[] = ['standard', 'cache']
const unitOptions: UnitMode[] = ['1m', '1k']

const allModels = computed<CatalogModelItem[]>(() =>
  providers.value.flatMap((provider) => provider.models.map((model) => ({ provider, model }))),
)

const totalModels = computed(() => allModels.value.length)

const tagOptions = computed(() => {
  const counts = new Map<string, number>()
  for (const item of allModels.value) {
    for (const tag of item.model.tags) {
      counts.set(tag, (counts.get(tag) || 0) + 1)
    }
  }
  return [...counts.entries()]
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name))
})

const filteredModels = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  const rows = allModels.value.filter((item) => {
    if (selectedProviderId.value !== 'all' && item.provider.id !== selectedProviderId.value) return false
    if (selectedTags.value.length && !selectedTags.value.every((tag) => item.model.tags.includes(tag))) return false
    if (selectedPriceField.value === 'input' && item.model.pricing.input === null) return false
    if (selectedPriceField.value === 'output' && item.model.pricing.output === null) return false
    if (selectedPriceField.value === 'cache' && item.model.pricing.cache_write === null && item.model.pricing.cache_read === null) return false
    if (!q) return true
    const haystack = [
      item.provider.display_name,
      item.model.id,
      item.model.name,
      item.model.description,
      ...item.model.tags,
    ].join(' ').toLowerCase()
    return haystack.includes(q)
  })

  return [...rows].sort((a, b) => {
    if (sortBy.value === 'input') return nullablePrice(a.model.pricing.input) - nullablePrice(b.model.pricing.input)
    if (sortBy.value === 'output') return nullablePrice(a.model.pricing.output) - nullablePrice(b.model.pricing.output)
    if (sortBy.value === 'context') return b.model.context_tokens - a.model.context_tokens
    return a.model.name.localeCompare(b.model.name)
  })
})

async function loadCatalog() {
  loading.value = true
  errorMessage.value = ''
  try {
    providers.value = await modelsCatalogAPI.getCatalog()
  } catch (err: unknown) {
    const message = extractApiErrorMessage(err, t('modelsCatalog.loadFailed'))
    providers.value = []
    errorMessage.value = message
    appStore.showError(message)
  } finally {
    loading.value = false
  }
}

function resetFilters() {
  searchQuery.value = ''
  selectedProviderId.value = 'all'
  selectedTags.value = []
  selectedPriceField.value = 'all'
  priceMode.value = 'standard'
  unitMode.value = '1m'
  sortBy.value = 'name'
}

function toggleTag(tag: string) {
  selectedTags.value = selectedTags.value.includes(tag)
    ? selectedTags.value.filter((item) => item !== tag)
    : [...selectedTags.value, tag]
}

function nullablePrice(value: number | null): number {
  return value === null ? Number.POSITIVE_INFINITY : value
}

function formatTag(tag: string): string {
  return t(`modelsCatalog.tags.${tag}`, tag)
}

function formatPrice(value: number | null): string {
  if (value === null) return t('modelsCatalog.priceUnavailable')
  const normalized = unitMode.value === '1k' ? value / 1000 : value
  const unit = unitMode.value === '1k' ? t('modelsCatalog.unitSuffix.1k') : t('modelsCatalog.unitSuffix.1m')
  return `￥${formatPriceNumber(normalized)} ${unit}`
}

function formatPriceNumber(value: number): string {
  if (value >= 1) return value.toFixed(2)
  if (value >= 0.01) return value.toFixed(3)
  return value.toFixed(5)
}

function formatTokenCount(value: number): string {
  if (value >= 1000000) return `${Number(value / 1000000).toLocaleString()}M`
  if (value >= 1000) return `${Number(value / 1000).toLocaleString()}K`
  return value.toLocaleString()
}

onMounted(loadCatalog)
</script>

<style scoped>
.models-catalog-page {
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 22px;
}

.catalog-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.catalog-heading h1 {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 10px;
  margin: 0;
  color: var(--app-text);
  font-size: clamp(1.45rem, 2vw, 2rem);
  font-weight: 760;
  letter-spacing: 0;
}

.catalog-heading p {
  margin: 6px 0 0;
  color: var(--app-text-muted);
}

.catalog-price-note {
  color: var(--app-amber);
  font-size: clamp(1.05rem, 1.55vw, 1.55rem);
  font-weight: 780;
}

.catalog-kicker {
  margin: 0 0 4px !important;
  color: var(--app-accent);
  font-size: 0.78rem;
  font-weight: 700;
  letter-spacing: 0;
  text-transform: uppercase;
}

.catalog-layout {
  display: grid;
  grid-template-columns: minmax(250px, 300px) minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}

.glass-panel {
  border: 1px solid color-mix(in srgb, var(--app-line) 86%, transparent);
  background:
    linear-gradient(135deg, color-mix(in srgb, var(--app-surface) 86%, transparent), color-mix(in srgb, var(--app-surface-muted) 70%, transparent)),
    var(--app-surface);
  box-shadow: var(--app-shadow-soft);
  backdrop-filter: blur(18px) saturate(1.08);
}

.catalog-filters {
  position: sticky;
  top: 86px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
}

.filter-block {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding-bottom: 14px;
  border-bottom: 1px solid color-mix(in srgb, var(--app-line) 74%, transparent);
}

.filter-block:last-child {
  padding-bottom: 0;
  border-bottom: 0;
}

.filter-title-row,
.filter-section-heading,
.catalog-toolbar,
.toolbar-controls,
.model-card-header,
.model-meta-row {
  display: flex;
  align-items: center;
}

.filter-title-row {
  justify-content: space-between;
  gap: 12px;
}

.filter-title-row h2 {
  margin: 0;
  color: var(--app-text);
  font-size: 0.98rem;
  font-weight: 740;
}

.filter-title-row p {
  margin: 3px 0 0;
  color: var(--app-text-muted);
  font-size: 0.78rem;
}

.search-icon {
  position: absolute;
  left: 12px;
  top: 50%;
  color: var(--app-text-muted);
  transform: translateY(-50%);
}

.catalog-search {
  padding-left: 40px;
}

.filter-section-heading {
  justify-content: space-between;
  width: 100%;
  color: var(--app-text);
  font-weight: 680;
  transition: color 140ms ease;
}

.filter-section-heading svg {
  transition: transform 140ms ease;
}

.chip-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.filter-chip {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-height: 30px;
  border: 1px solid var(--app-line);
  background: color-mix(in srgb, var(--app-surface) 82%, transparent);
  color: var(--app-text-muted);
  padding: 5px 9px;
  font-size: 0.78rem;
  font-weight: 640;
  transition: transform 140ms ease, border-color 140ms ease, background 140ms ease, color 140ms ease;
}

.filter-chip:hover {
  transform: translateY(-1px);
  border-color: color-mix(in srgb, var(--app-accent) 42%, var(--app-line));
  color: var(--app-text);
}

.filter-chip.active {
  border-color: color-mix(in srgb, var(--app-accent) 72%, var(--app-line));
  background: color-mix(in srgb, var(--app-accent) 13%, var(--app-surface));
  color: var(--app-accent-strong);
}

.filter-chip strong {
  color: inherit;
  font-size: 0.72rem;
  opacity: 0.78;
}

.catalog-main {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.catalog-toolbar {
  justify-content: space-between;
  gap: 14px;
  min-height: 58px;
  padding: 10px 12px;
}

.result-count {
  display: inline-flex;
  align-items: baseline;
  gap: 7px;
  color: var(--app-text-muted);
  white-space: nowrap;
}

.result-count strong {
  color: var(--app-text);
  font-size: 1.15rem;
}

.toolbar-controls {
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.segmented,
.sort-select {
  display: inline-flex;
  align-items: center;
  min-height: 34px;
  border: 1px solid var(--app-line);
  background: color-mix(in srgb, var(--app-surface) 86%, transparent);
}

.segmented button {
  min-height: 32px;
  padding: 0 11px;
  color: var(--app-text-muted);
  font-size: 0.8rem;
  font-weight: 700;
  transition: background 140ms ease, color 140ms ease;
}

.segmented button.active {
  background: var(--app-accent);
  color: var(--app-on-accent);
}

.sort-select {
  gap: 7px;
  padding: 0 8px;
  color: var(--app-text-muted);
}

.sort-select select {
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--app-text);
  font-size: 0.8rem;
  font-weight: 700;
}

.models-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.model-card {
  position: relative;
  min-height: 258px;
  padding: 16px;
  overflow: hidden;
  transition: transform 150ms ease, border-color 150ms ease, box-shadow 150ms ease;
}

.model-card::after {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(110deg, transparent 12%, color-mix(in srgb, #fff 20%, transparent) 42%, transparent 72%);
  opacity: 0;
  transform: translateX(-35%);
  transition: opacity 160ms ease, transform 320ms ease;
}

.model-card:hover {
  transform: translateY(-2px);
  border-color: color-mix(in srgb, var(--app-accent) 36%, var(--app-line));
  box-shadow: var(--app-shadow-medium);
}

.model-card:hover::after {
  opacity: 1;
  transform: translateX(35%);
}

.model-card-header {
  gap: 12px;
}

.model-card-header h2 {
  margin: 0;
  overflow: hidden;
  color: var(--app-text);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.95rem;
  font-weight: 760;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.model-card-header p {
  margin: 4px 0 0;
  color: var(--app-text-muted);
  font-size: 0.77rem;
}

.model-mark,
.icon-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--app-line);
  background: color-mix(in srgb, var(--app-surface-muted) 78%, transparent);
}

.model-mark {
  width: 40px;
  height: 40px;
  flex: 0 0 auto;
}

.icon-button {
  width: 32px;
  height: 32px;
  margin-left: auto;
  color: var(--app-text-muted);
  transition: transform 140ms ease, border-color 140ms ease, color 140ms ease, background 140ms ease;
}

.icon-button:hover {
  transform: translateY(-1px);
  border-color: color-mix(in srgb, var(--app-accent) 48%, var(--app-line));
  background: color-mix(in srgb, var(--app-accent) 11%, var(--app-surface));
  color: var(--app-accent-strong);
}

.model-description {
  min-height: 44px;
  margin: 15px 0 0;
  color: var(--app-text-muted);
  font-size: 0.84rem;
  line-height: 1.55;
}

.tag-row {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
  margin-top: 14px;
}

.tag-pill {
  border: 1px solid color-mix(in srgb, var(--app-line) 80%, transparent);
  background: color-mix(in srgb, var(--app-surface-muted) 70%, transparent);
  color: var(--app-text-muted);
  padding: 3px 7px;
  font-size: 0.72rem;
  font-weight: 660;
}

.pricing-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-top: 16px;
}

.price-cell {
  border: 1px solid color-mix(in srgb, var(--app-line) 78%, transparent);
  background: color-mix(in srgb, var(--app-surface-muted) 64%, transparent);
  padding: 9px;
}

.price-cell.primary,
.pricing-grid.cache .price-cell:not(.primary) {
  background: color-mix(in srgb, var(--app-accent) 9%, var(--app-surface));
}

.pricing-grid.cache .price-cell.primary {
  background: color-mix(in srgb, var(--app-surface-muted) 64%, transparent);
}

.price-cell span {
  display: block;
  color: var(--app-text-muted);
  font-size: 0.72rem;
}

.price-cell strong {
  display: block;
  margin-top: 4px;
  color: var(--app-text);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.86rem;
  font-weight: 760;
}

.model-meta-row {
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 15px;
  color: var(--app-text-muted);
  font-size: 0.74rem;
}

.model-meta-row span {
  border-left: 2px solid color-mix(in srgb, var(--app-accent) 42%, var(--app-line));
  padding-left: 8px;
}

.empty-panel {
  display: flex;
  min-height: 360px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: var(--app-text-muted);
  text-align: center;
}

.empty-panel h2 {
  margin: 0;
  color: var(--app-text);
  font-size: 1.1rem;
}

.empty-panel p {
  margin: 0 0 8px;
}

.error-panel svg {
  color: var(--app-danger);
}

.skeleton-card {
  border: 1px solid var(--app-line);
  background: color-mix(in srgb, var(--app-surface) 82%, transparent);
}

.skeleton-line,
.skeleton-metrics {
  height: 14px;
  margin-bottom: 14px;
  background: linear-gradient(90deg, var(--app-surface-muted), color-mix(in srgb, var(--app-surface-muted) 60%, #fff), var(--app-surface-muted));
  background-size: 220% 100%;
  animation: shimmer 1.2s ease-in-out infinite;
}

.skeleton-metrics {
  height: 92px;
  margin-top: 26px;
}

@keyframes shimmer {
  to {
    background-position: -220% 0;
  }
}

@media (max-width: 1180px) {
  .models-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 900px) {
  .models-catalog-page {
    padding: 14px;
  }

  .catalog-heading,
  .catalog-toolbar {
    flex-direction: column;
    align-items: stretch;
  }

  .catalog-layout {
    grid-template-columns: 1fr;
  }

  .catalog-filters {
    position: static;
  }

  .toolbar-controls {
    justify-content: flex-start;
  }
}

@media (max-width: 620px) {
  .models-grid {
    grid-template-columns: 1fr;
  }

  .segmented,
  .sort-select {
    width: 100%;
  }

  .segmented button {
    flex: 1 1 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .filter-chip,
  .icon-button,
  .model-card,
  .model-card::after,
  .skeleton-line,
  .skeleton-metrics {
    animation: none;
    transition: none;
  }
}
</style>
