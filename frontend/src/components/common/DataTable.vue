<template>
  <div v-if="!isDesktopViewport" class="space-y-3">
    <template v-if="loading">
      <div v-for="i in 5" :key="i" class="mobile-table-card p-4">
        <div class="space-y-3">
          <div v-for="column in dataColumns" :key="column.key" class="flex justify-between">
            <div class="liquid-skeleton h-4 w-20 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
            <div class="liquid-skeleton h-4 w-32 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
          </div>
          <div v-if="hasActionsColumn" class="border-t border-gray-200 pt-3 dark:border-dark-700">
            <div class="liquid-skeleton h-8 w-full animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
          </div>
        </div>
      </div>
    </template>

    <template v-else-if="!data || data.length === 0">
      <div class="mobile-table-empty p-12 text-center">
        <slot name="empty">
          <div class="flex flex-col items-center">
            <Icon
              name="inbox"
              size="xl"
              class="mb-4 h-12 w-12 text-gray-400 dark:text-dark-500"
            />
            <p class="text-lg font-medium text-gray-900 dark:text-gray-100">
              {{ t('empty.noData') }}
            </p>
          </div>
        </slot>
      </div>
    </template>

    <template v-else>
      <div
        v-for="(row, index) in sortedData"
        :key="resolveRowKey(row, index)"
        class="mobile-table-card p-4"
      >
        <div class="space-y-3">
          <div
            v-for="column in dataColumns"
            :key="column.key"
            class="flex items-start justify-between gap-4"
          >
            <span class="text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">
              {{ column.label }}
            </span>
            <div class="text-right text-sm text-gray-900 dark:text-gray-100">
              <slot :name="`cell-${column.key}`" :row="row" :value="row[column.key]" :expanded="actionsExpanded">
                {{ column.formatter ? column.formatter(row[column.key], row) : row[column.key] }}
              </slot>
            </div>
          </div>
          <div v-if="hasActionsColumn" class="border-t border-gray-200 pt-3 dark:border-dark-700">
            <slot name="cell-actions" :row="row" :value="row['actions']" :expanded="actionsExpanded"></slot>
          </div>
        </div>
      </div>
    </template>
  </div>

  <div
    v-else
    ref="tableWrapperRef"
    class="table-wrapper"
    :class="{
      'actions-expanded': actionsExpanded,
      'is-scrollable': isScrollable
    }"
  >
    <table class="w-full min-w-max divide-y divide-gray-200/75 dark:divide-dark-700">
      <thead class="table-header bg-gray-50/80 dark:bg-dark-800">
        <tr>
          <th
            v-for="(column, index) in columns"
            :key="column.key"
            scope="col"
            :class="[
              'sticky-header-cell py-3 text-left text-xs font-bold uppercase tracking-[0.04em] text-gray-600 dark:text-dark-300',
              getAdaptivePaddingClass(),
              { 'cursor-pointer hover:text-gray-950 dark:hover:text-white': column.sortable },
              getStickyColumnClass(column, index),
              column.class
            ]"
            @click="column.sortable && handleSort(column.key)"
          >
            <slot
              :name="`header-${column.key}`"
              :column="column"
              :sort-key="sortKey"
              :sort-order="sortOrder"
            >
              <div class="table-header-label flex items-center space-x-1">
                <span>{{ column.label }}</span>
                <span v-if="column.sortable" class="sort-icon text-gray-400 dark:text-dark-500">
                  <svg
                    v-if="sortKey === column.key"
                    class="h-4 w-4"
                    :class="{ 'rotate-180 transform': sortOrder === 'desc' }"
                    fill="currentColor"
                    viewBox="0 0 20 20"
                  >
                    <path
                      fill-rule="evenodd"
                      d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z"
                      clip-rule="evenodd"
                    />
                  </svg>
                  <svg v-else class="h-4 w-4" fill="currentColor" viewBox="0 0 20 20">
                    <path
                      d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z"
                    />
                  </svg>
                </span>
              </div>
            </slot>
          </th>
        </tr>
      </thead>
      <tbody class="table-body divide-y divide-gray-200/75 bg-white/80 dark:divide-dark-700 dark:bg-dark-900">
        <!-- Loading skeleton -->
        <tr v-if="loading" v-for="i in 5" :key="i">
          <td v-for="column in columns" :key="column.key" :class="['whitespace-nowrap py-4', getAdaptivePaddingClass()]">
            <div class="animate-pulse">
              <div class="liquid-skeleton h-4 w-3/4 rounded bg-gray-200 dark:bg-dark-700"></div>
            </div>
          </td>
        </tr>

        <!-- Empty state -->
        <tr v-else-if="!data || data.length === 0">
          <td
            :colspan="columns.length"
            :class="['py-12 text-center text-gray-500 dark:text-dark-400', getAdaptivePaddingClass()]"
          >
            <slot name="empty">
              <div class="flex flex-col items-center">
                <Icon
                  name="inbox"
                  size="xl"
                  class="mb-4 h-12 w-12 text-gray-400 dark:text-dark-500"
                />
                <p class="text-lg font-medium text-gray-900 dark:text-gray-100">
                  {{ t('empty.noData') }}
                </p>
              </div>
            </slot>
          </td>
        </tr>

        <!-- Data rows (virtual scroll) -->
        <template v-else>
          <tr v-if="virtualPaddingTop > 0" aria-hidden="true">
            <td :colspan="columns.length"
                :style="{ height: virtualPaddingTop + 'px', padding: 0, border: 'none' }">
            </td>
          </tr>
          <tr
            v-for="virtualRow in virtualItems"
            :key="resolveRowKey(sortedData[virtualRow.index], virtualRow.index)"
            :data-row-id="resolveRowKey(sortedData[virtualRow.index], virtualRow.index)"
            :data-index="virtualRow.index"
            :ref="measureElement"
            class="table-row-hover"
          >
            <td
              v-for="(column, colIndex) in columns"
              :key="column.key"
              :class="[
                'whitespace-nowrap py-4 text-sm text-gray-900 dark:text-gray-100',
                getAdaptivePaddingClass(),
                getStickyColumnClass(column, colIndex),
                column.class
              ]"
            >
              <slot :name="`cell-${column.key}`"
                    :row="sortedData[virtualRow.index]"
                    :value="sortedData[virtualRow.index][column.key]"
                    :expanded="actionsExpanded">
                {{ column.formatter
                   ? column.formatter(sortedData[virtualRow.index][column.key], sortedData[virtualRow.index])
                   : sortedData[virtualRow.index][column.key] }}
              </slot>
            </td>
          </tr>
          <tr v-if="virtualPaddingBottom > 0" aria-hidden="true">
            <td :colspan="columns.length"
                :style="{ height: virtualPaddingBottom + 'px', padding: 0, border: 'none' }">
            </td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useVirtualizer, observeElementRect as observeElementRectDefault } from '@tanstack/vue-virtual'
import { useI18n } from 'vue-i18n'
import type { Column } from './types'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

const desktopViewportQuery = '(min-width: 768px)'
const isDesktopViewport = ref(
  typeof window === 'undefined' ? true : window.matchMedia(desktopViewportQuery).matches
)

const emit = defineEmits<{
  sort: [key: string, order: 'asc' | 'desc']
}>()

// 表格容器引用
const tableWrapperRef = ref<HTMLElement | null>(null)
const isScrollable = ref(false)
const actionsColumnNeedsExpanding = ref(false)

// --- 虚拟滚动「整表空白」根治 ---
// 根因:本组件根 .table-wrapper 为 flex:1 / min-h-0,高度由父级 flex 链决定。@tanstack 虚拟化器
// 仅在 observeElementRect 回调里写 scrollRect;一旦该回调读到 0 高度(加载瞬间 flex 未结算,或
// 滚动中动态行高校正触发的 reflow),scrollRect 被钉死为 0 → calculateRange 返回 null → 整表空白。
// 对策(见下方 virtualizer 选项):
//   1) 覆写 observeElementRect,直接丢弃 height<=0 的读数,scrollRect 永不被钉成 0;
//   2) initialRect 给一屏兜底高度,首个有效读数到来前也有行可渲染,绝不空白。
// 兜底高度:表格区域大致 = 视口高度 - 顶栏/外边距/筛选/分页 ≈ 320px
const estimatedViewportHeight = () => {
  if (typeof window === 'undefined') return 600
  return Math.max(window.innerHeight - 320, 400)
}

// 覆写默认 observeElementRect:过滤掉 0 高度读数(根治整表空白的关键)
const observeElementRectNonZero = (
  instance: any,
  cb: (rect: { width: number; height: number }) => void
) => observeElementRectDefault(instance, (rect) => {
  if (rect.height > 0) cb(rect)
})

// 检查是否可滚动
const checkScrollable = () => {
  if (tableWrapperRef.value) {
    isScrollable.value = tableWrapperRef.value.scrollWidth > tableWrapperRef.value.clientWidth
  }
}

// 检查操作列是否需要展开
const checkActionsColumnWidth = () => {
  if (!tableWrapperRef.value) return

  // 查找第一行的操作列单元格
  const firstActionCell = tableWrapperRef.value.querySelector('tbody tr:first-child td:last-child')
  if (!firstActionCell) return

  // 查找操作列内容的容器div
  const actionsContainer = firstActionCell.querySelector('div')
  if (!actionsContainer) return

  // 临时展开以测量完整宽度
  const wasExpanded = actionsExpanded.value
  actionsExpanded.value = true

  // 等待DOM更新
  nextTick(() => {
    // 测量所有按钮的总宽度
    const actionItems = actionsContainer.querySelectorAll('button, a, [role="button"]')
    if (actionItems.length <= 2) {
      actionsColumnNeedsExpanding.value = false
      actionsExpanded.value = wasExpanded
      return
    }

    // 计算所有按钮的总宽度（包括gap）
    let totalWidth = 0
    actionItems.forEach((item, index) => {
      totalWidth += (item as HTMLElement).offsetWidth
      if (index < actionItems.length - 1) {
        totalWidth += 4 // gap-1 = 4px
      }
    })

    // 获取单元格可用宽度（减去padding）
    const cellWidth = (firstActionCell as HTMLElement).clientWidth - 32 // 减去左右padding

    // 如果总宽度超过可用宽度，需要展开功能
    actionsColumnNeedsExpanding.value = totalWidth > cellWidth

    // 恢复原来的展开状态
    actionsExpanded.value = wasExpanded
  })
}

// 监听尺寸变化
let resizeObserver: ResizeObserver | null = null
let resizeHandler: (() => void) | null = null
let desktopViewportMediaQuery: MediaQueryList | null = null
let desktopViewportListener: ((event: MediaQueryListEvent) => void) | null = null

const detachDesktopTableTracking = () => {
  resizeObserver?.disconnect()
  resizeObserver = null
  if (resizeHandler) {
    window.removeEventListener('resize', resizeHandler)
    resizeHandler = null
  }
}

const attachDesktopTableTracking = () => {
  checkScrollable()
  checkActionsColumnWidth()
  if (tableWrapperRef.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => {
      checkScrollable()
      checkActionsColumnWidth()
    })
    resizeObserver.observe(tableWrapperRef.value)
  } else {
    // 降级方案：不支持 ResizeObserver 时使用 window resize
    resizeHandler = () => {
      checkScrollable()
      checkActionsColumnWidth()
    }
    window.addEventListener('resize', resizeHandler)
  }
}

onMounted(() => {
  if (typeof window !== 'undefined') {
    desktopViewportMediaQuery = window.matchMedia(desktopViewportQuery)
    isDesktopViewport.value = desktopViewportMediaQuery.matches
    desktopViewportListener = (event: MediaQueryListEvent) => {
      isDesktopViewport.value = event.matches
    }
    if (typeof desktopViewportMediaQuery.addEventListener === 'function') {
      desktopViewportMediaQuery.addEventListener('change', desktopViewportListener)
    } else {
      desktopViewportMediaQuery.addListener(desktopViewportListener)
    }
  }
})

onUnmounted(() => {
  detachDesktopTableTracking()
  if (desktopViewportMediaQuery && desktopViewportListener) {
    if (typeof desktopViewportMediaQuery.removeEventListener === 'function') {
      desktopViewportMediaQuery.removeEventListener('change', desktopViewportListener)
    } else {
      desktopViewportMediaQuery.removeListener(desktopViewportListener)
    }
    desktopViewportListener = null
  }
  desktopViewportMediaQuery = null
})

interface Props {
  columns: Column[]
  data: any[]
  loading?: boolean
  stickyFirstColumn?: boolean
  stickyActionsColumn?: boolean
  expandableActions?: boolean
  actionsCount?: number // 操作按钮总数，用于判断是否需要展开功能
  rowKey?: string | ((row: any) => string | number)
  /**
   * Default sort configuration (only applied when there is no persisted sort state)
   */
  defaultSortKey?: string
  defaultSortOrder?: 'asc' | 'desc'
  /**
   * Persist sort state (key + order) to localStorage using this key.
   * If provided, DataTable will load the stored sort state on mount.
   */
  sortStorageKey?: string
  /**
   * Enable server-side sorting mode. When true, clicking sort headers
   * will emit 'sort' events instead of performing client-side sorting.
   */
  serverSideSort?: boolean
  /** Estimated row height in px for the virtualizer (default 56) */
  estimateRowHeight?: number
  /** Number of rows to render beyond the visible area (default 5) */
  overscan?: number
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  stickyFirstColumn: true,
  stickyActionsColumn: true,
  expandableActions: true,
  defaultSortOrder: 'asc',
  serverSideSort: false
})

const sortKey = ref<string>('')
const sortOrder = ref<'asc' | 'desc'>('asc')
const actionsExpanded = ref(false)

type PersistedSortState = {
  key: string
  order: 'asc' | 'desc'
}

const collator = new Intl.Collator(undefined, {
  numeric: true,
  sensitivity: 'base'
})

const getSortableKeys = () => {
  const keys = new Set<string>()
  for (const col of props.columns) {
    if (col.sortable) keys.add(col.key)
  }
  return keys
}

const normalizeSortKey = (candidate: string) => {
  if (!candidate) return ''
  const sortableKeys = getSortableKeys()
  return sortableKeys.has(candidate) ? candidate : ''
}

const normalizeSortOrder = (candidate: any): 'asc' | 'desc' => {
  return candidate === 'desc' ? 'desc' : 'asc'
}

const readPersistedSortState = (): PersistedSortState | null => {
  if (!props.sortStorageKey) return null
  try {
    const raw = localStorage.getItem(props.sortStorageKey)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<PersistedSortState>
    const key = normalizeSortKey(typeof parsed.key === 'string' ? parsed.key : '')
    if (!key) return null
    return { key, order: normalizeSortOrder(parsed.order) }
  } catch (e) {
    console.error('[DataTable] Failed to read persisted sort state:', e)
    return null
  }
}

const writePersistedSortState = (state: PersistedSortState) => {
  if (!props.sortStorageKey) return
  try {
    localStorage.setItem(props.sortStorageKey, JSON.stringify(state))
  } catch (e) {
    console.error('[DataTable] Failed to persist sort state:', e)
  }
}

const resolveInitialSortState = (): PersistedSortState | null => {
  const persisted = readPersistedSortState()
  if (persisted) return persisted

  const key = normalizeSortKey(props.defaultSortKey || '')
  if (!key) return null
  return { key, order: normalizeSortOrder(props.defaultSortOrder) }
}

const applySortState = (state: PersistedSortState | null) => {
  if (!state) return
  sortKey.value = state.key
  sortOrder.value = state.order
}

const isNullishOrEmpty = (value: any) => value === null || value === undefined || value === ''

const toFiniteNumberOrNull = (value: any): number | null => {
  if (typeof value === 'number') return Number.isFinite(value) ? value : null
  if (typeof value === 'boolean') return value ? 1 : 0
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed) return null
    const n = Number(trimmed)
    return Number.isFinite(n) ? n : null
  }
  return null
}

const toSortableString = (value: any): string => {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (value instanceof Date) return value.toISOString()
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

const compareSortValues = (a: any, b: any): number => {
  const aEmpty = isNullishOrEmpty(a)
  const bEmpty = isNullishOrEmpty(b)
  if (aEmpty && bEmpty) return 0
  if (aEmpty) return 1
  if (bEmpty) return -1

  const aNum = toFiniteNumberOrNull(a)
  const bNum = toFiniteNumberOrNull(b)
  if (aNum !== null && bNum !== null) {
    if (aNum === bNum) return 0
    return aNum < bNum ? -1 : 1
  }

  const aStr = toSortableString(a)
  const bStr = toSortableString(b)
  const res = collator.compare(aStr, bStr)
  if (res === 0) return 0
  return res < 0 ? -1 : 1
}
const resolveRowKey = (row: any, index: number) => {
  if (typeof props.rowKey === 'function') {
    const key = props.rowKey(row)
    return key ?? index
  }
  if (typeof props.rowKey === 'string' && props.rowKey) {
    const key = row?.[props.rowKey]
    return key ?? index
  }
  const key = row?.id
  return key ?? index
}

const dataColumns = computed(() => props.columns.filter((column) => column.key !== 'actions'))
const columnsSignature = computed(() =>
  props.columns.map((column) => `${column.key}:${column.sortable ? '1' : '0'}`).join('|')
)

watch(
  isDesktopViewport,
  async (isDesktop) => {
    detachDesktopTableTracking()
    if (!isDesktop) return
    await nextTick()
    attachDesktopTableTracking()
  },
  { immediate: true, flush: 'post' }
)

// 数据/列变化时重新检查滚动状态
// 注意：不能监听 actionsExpanded，因为 checkActionsColumnWidth 会临时修改它，会导致无限循环
watch(
  [() => props.data.length, columnsSignature],
  async () => {
    await nextTick()
    checkScrollable()
    checkActionsColumnWidth()
  },
  { flush: 'post' }
)

// 单独监听展开状态变化，只更新滚动状态
watch(actionsExpanded, async () => {
  await nextTick()
  checkScrollable()
})

const handleSort = (key: string) => {
  let newOrder: 'asc' | 'desc' = 'asc'
  if (sortKey.value === key) {
    newOrder = sortOrder.value === 'asc' ? 'desc' : 'asc'
  }

  if (props.serverSideSort) {
    // Server-side sort mode: emit event and update internal state for UI feedback
    sortKey.value = key
    sortOrder.value = newOrder
    emit('sort', key, newOrder)
  } else {
    // Client-side sort mode: just update internal state
    sortKey.value = key
    sortOrder.value = newOrder
  }
}

const sortedData = computed(() => {
  // Server-side sort mode: return data as-is (server handles sorting)
  if (props.serverSideSort || !sortKey.value || !props.data) return props.data

  const key = sortKey.value
  const order = sortOrder.value

  // Stable sort (tie-break with original index) to avoid jitter when values are equal.
  return props.data
    .map((row, index) => ({ row, index }))
    .sort((a, b) => {
      const cmp = compareSortValues(a.row?.[key], b.row?.[key])
      if (cmp !== 0) return order === 'asc' ? cmp : -cmp
      return a.index - b.index
    })
    .map(item => item.row)
})

// --- Virtual scrolling ---
const rowVirtualizer = useVirtualizer(computed(() => ({
  count: isDesktopViewport.value ? (sortedData.value?.length ?? 0) : 0,
  getScrollElement: () => tableWrapperRef.value,
  estimateSize: () => props.estimateRowHeight ?? 56,
  overscan: props.overscan ?? 5,
  // 兜底高度:首个有效高度读数到来前,先按一屏渲染,避免空白帧
  initialRect: { width: 0, height: estimatedViewportHeight() },
  // 关键:过滤 0 高度读数,杜绝 scrollRect 被钉成 0 → calculateRange 返回 null → 整表空白
  observeElementRect: observeElementRectNonZero,
  // 把测量类 ResizeObserver 回调批到 rAF,避免滚动中同步 reflow 风暴导致的校正抖动/空白
  useAnimationFrameWithResizeObserver: true,
})))

const virtualItems = computed(() => rowVirtualizer.value.getVirtualItems())

const virtualPaddingTop = computed(() => {
  const items = virtualItems.value
  return items.length > 0 ? items[0].start : 0
})

const virtualPaddingBottom = computed(() => {
  const items = virtualItems.value
  if (items.length === 0) return 0
  return rowVirtualizer.value.getTotalSize() - items[items.length - 1].end
})

const measureElement = (el: any) => {
  if (el) {
    rowVirtualizer.value.measureElement(el as Element)
  }
}

const hasActionsColumn = computed(() => {
  return props.columns.some(column => column.key === 'actions')
})

const hasSelectColumn = computed(() => {
  return props.columns.length > 0 && props.columns[0].key === 'select'
})

// 生成固定列的 CSS 类
const getStickyColumnClass = (column: Column, index: number) => {
  const classes: string[] = []

  if (props.stickyFirstColumn) {
    // 如果第一列是勾选列，固定前两列（勾选+名称）
    if (hasSelectColumn.value) {
      if (index === 0) {
        classes.push('sticky-col sticky-col-left-first')
      } else if (index === 1) {
        classes.push('sticky-col sticky-col-left-second')
      }
    } else {
      // 否则只固定第一列
      if (index === 0) {
        classes.push('sticky-col sticky-col-left')
      }
    }
  }

  // 操作列固定（最后一列）
  if (props.stickyActionsColumn && column.key === 'actions') {
    classes.push('sticky-col sticky-col-right')
  }

  return classes.join(' ')
}

// 根据列数自适应调整内边距
const getAdaptivePaddingClass = () => {
  const columnCount = props.columns.length

  // 列数越多，内边距越小
  if (columnCount >= 10) {
    return 'px-2' // 8px
  } else if (columnCount >= 7) {
    return 'px-3' // 12px
  } else if (columnCount >= 5) {
    return 'px-4' // 16px
  } else {
    return 'px-6' // 24px (原始值)
  }
}

// Init + keep persisted sort state consistent with current columns
const didInitSort = ref(false)

onMounted(() => {
  const initial = resolveInitialSortState()
  applySortState(initial)
  didInitSort.value = true
})

watch(
  columnsSignature,
  () => {
    // If current sort key is no longer sortable/visible, fall back to default/persisted.
    const normalized = normalizeSortKey(sortKey.value)
    if (!sortKey.value) {
      const initial = resolveInitialSortState()
      applySortState(initial)
      return
    }

    if (!normalized) {
      const fallback = resolveInitialSortState()
      if (fallback) {
        applySortState(fallback)
      } else {
        sortKey.value = ''
        sortOrder.value = 'asc'
      }
    }
  },
  { flush: 'post' }
)

watch(
  [sortKey, sortOrder],
  ([nextKey, nextOrder]) => {
    if (!didInitSort.value) return
    if (!props.sortStorageKey) return
    const key = normalizeSortKey(nextKey)
    if (!key) return
    writePersistedSortState({ key, order: normalizeSortOrder(nextOrder) })
  },
  { flush: 'post' }
)

defineExpose({
  virtualizer: rowVirtualizer,
  sortedData,
  resolveRowKey,
  tableWrapperEl: tableWrapperRef,
})
</script>

<style scoped>
.table-wrapper {
  --table-surface: var(--app-surface);
  --table-surface-strong: var(--app-surface-2);
  --table-surface-hover: color-mix(in srgb, var(--app-brand) 8%, var(--app-surface));
  --table-border: var(--app-line);
  --table-highlight: color-mix(in srgb, var(--app-brand) 18%, transparent);
  --select-col-width: 52px;
  position: relative;
  overflow-x: auto;
  overflow-y: auto;
  flex: 1;
  min-height: 0;
  isolation: isolate;
  border: 1px solid var(--table-border);
  border-radius: var(--ui-radius);
  background: var(--table-surface);
  box-shadow: var(--app-shadow);
  transition:
    border-color 150ms ease,
    box-shadow 150ms ease,
    background-color 150ms ease;
}

.table-wrapper:hover,
.table-wrapper:focus-within {
  border-color: var(--app-line-strong);
  box-shadow: var(--app-shadow-strong);
}

.table-wrapper .table-header {
  position: sticky;
  top: 0;
  z-index: 200;
  background: var(--table-surface-strong);
  box-shadow: inset 0 -1px 0 var(--table-border);
}

.table-body {
  position: relative;
  z-index: 0;
  background: var(--table-surface);
}

.table-row-hover {
  position: relative;
  transition:
    background-color 150ms ease,
    box-shadow 150ms ease;
}

.table-row-hover:hover {
  background: var(--table-surface-hover);
  box-shadow: inset 2px 0 0 var(--app-brand);
}

.liquid-skeleton {
  position: relative;
  overflow: hidden;
  background-color: var(--app-surface-3) !important;
}

.liquid-skeleton::after {
  content: '';
  position: absolute;
  inset: 0;
  transform: translateX(-100%);
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.52), transparent);
  animation: table-shimmer 1.4s ease-in-out infinite;
}

.dark .liquid-skeleton::after {
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.08), transparent);
}

.mobile-table-card,
.mobile-table-empty {
  border: 1px solid var(--app-line);
  border-radius: var(--ui-radius);
  background: var(--app-surface);
  box-shadow: var(--app-shadow);
  transition:
    border-color 150ms ease,
    box-shadow 150ms ease,
    background-color 150ms ease;
}

.mobile-table-card:hover {
  border-color: var(--app-line-strong);
  box-shadow: var(--app-shadow-strong);
}

@keyframes table-shimmer {
  100% {
    transform: translateX(100%);
  }
}

.sticky-header-cell {
  position: sticky;
  top: 0;
  z-index: 210;
  background: transparent;
  transition:
    color 150ms ease,
    background-color 150ms ease;
}

.sticky-header-cell:hover {
  background-color: color-mix(in srgb, var(--app-brand) 10%, transparent);
}

.table-header-label {
  transition: color 150ms ease;
}

.sort-icon svg {
  transition:
    color 150ms ease,
    transform 150ms ease,
    opacity 150ms ease;
}

.sticky-header-cell:hover .sort-icon {
  color: var(--app-brand-strong);
}

.sticky-col {
  position: sticky;
  z-index: 20;
}

.sticky-col-left,
.sticky-col-left-first {
  left: 0;
}

.sticky-col-left-second {
  left: var(--select-col-width);
}

.sticky-col-right {
  right: 0;
}

.sticky-header-cell.sticky-col {
  z-index: 220;
}

tbody .sticky-col {
  background-color: var(--table-surface);
}

tbody tr:hover .sticky-col {
  background-color: var(--table-surface-hover);
}

.is-scrollable .sticky-col-left::after,
.is-scrollable .sticky-col-left-second::after,
.is-scrollable .sticky-col-right::before {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  width: 10px;
  pointer-events: none;
}

.is-scrollable .sticky-col-left::after,
.is-scrollable .sticky-col-left-second::after {
  right: 0;
  transform: translateX(100%);
  background: linear-gradient(to right, rgba(23, 26, 31, 0.12), transparent);
}

.is-scrollable .sticky-col-right::before {
  left: 0;
  transform: translateX(-100%);
  background: linear-gradient(to left, rgba(23, 26, 31, 0.12), transparent);
}

.dark .is-scrollable .sticky-col-left::after,
.dark .is-scrollable .sticky-col-left-second::after {
  background: linear-gradient(to right, rgba(0, 0, 0, 0.28), transparent);
}

.dark .is-scrollable .sticky-col-right::before {
  background: linear-gradient(to left, rgba(0, 0, 0, 0.28), transparent);
}

@media (prefers-reduced-motion: reduce) {
  .table-wrapper,
  .table-row-hover,
  .sticky-header-cell,
  .table-header-label,
  .sort-icon svg,
  .mobile-table-card,
  .mobile-table-empty {
    transition-duration: 1ms;
  }

  .liquid-skeleton::after {
    animation: none;
  }
}
</style>

<style>
.table-wrapper {
  scrollbar-width: thin !important;
  scrollbar-color: color-mix(in srgb, var(--app-muted) 55%, transparent) transparent !important;
}

.table-wrapper::-webkit-scrollbar {
  height: 12px !important;
  width: 12px !important;
  display: block !important;
  background-color: transparent !important;
}

.table-wrapper::-webkit-scrollbar-track {
  background-color: transparent !important;
  margin: 0 4px !important;
}

.table-wrapper::-webkit-scrollbar-thumb {
  border: 3px solid transparent !important;
  border-radius: var(--ui-radius) !important;
  background-color: color-mix(in srgb, var(--app-muted) 58%, transparent) !important;
  background-clip: padding-box !important;
}

.table-wrapper::-webkit-scrollbar-thumb:hover {
  background-color: color-mix(in srgb, var(--app-muted) 82%, transparent) !important;
}
</style>
