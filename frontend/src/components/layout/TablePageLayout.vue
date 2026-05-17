<template>
  <div class="table-page-layout" :class="{ 'mobile-mode': isMobile }">
    <div v-if="$slots.actions" class="layout-section-fixed">
      <slot name="actions" />
    </div>

    <div v-if="$slots.filters" class="layout-section-fixed">
      <slot name="filters" />
    </div>

    <div class="layout-section-scrollable">
      <div class="card table-scroll-container">
        <slot name="table" />
      </div>
    </div>

    <div v-if="$slots.pagination" class="layout-section-fixed">
      <slot name="pagination" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'

const isMobile = ref(false)

const checkMobile = () => {
  isMobile.value = window.innerWidth < 1024
}

onMounted(() => {
  checkMobile()
  window.addEventListener('resize', checkMobile)
})

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile)
})
</script>

<style scoped>
.table-page-layout {
  display: flex;
  min-width: 0;
  max-width: 100%;
  flex-direction: column;
  gap: 1rem;
  height: calc(100vh - 64px - 3rem);
}

.layout-section-fixed {
  min-width: 0;
  max-width: 100%;
  flex-shrink: 0;
}

.layout-section-scrollable {
  display: flex;
  min-height: 0;
  min-width: 0;
  max-width: 100%;
  flex: 1;
  flex-direction: column;
}

.table-scroll-container {
  display: flex;
  height: 100%;
  min-width: 0;
  max-width: 100%;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--app-line);
  border-radius: var(--ui-radius);
  background: var(--app-surface);
  box-shadow: var(--app-shadow);
}

.table-scroll-container :deep(.table-wrapper) {
  flex: 1;
  overflow-x: auto;
  overflow-y: auto;
  scrollbar-gutter: stable;
}

.table-scroll-container :deep(table) {
  width: 100%;
  min-width: max-content;
  display: table;
}

.table-scroll-container :deep(thead) {
  background: var(--app-surface-2);
}

.table-scroll-container :deep(th) {
  border-bottom: 1px solid var(--app-line);
  color: var(--app-muted);
  padding: 0.75rem 1.25rem;
  text-align: left;
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.table-scroll-container :deep(td) {
  border-bottom: 1px solid var(--app-line);
  color: var(--app-text-soft);
  padding: 0.75rem 1.25rem;
  font-size: 0.875rem;
}

.table-page-layout.mobile-mode .table-scroll-container {
  height: auto;
  overflow: hidden;
}

.table-page-layout.mobile-mode .layout-section-scrollable {
  min-height: 0;
  flex: none;
}

.table-page-layout.mobile-mode .table-scroll-container :deep(.table-wrapper) {
  overflow-x: auto;
  overflow-y: hidden;
}

.table-page-layout.mobile-mode .table-scroll-container :deep(table) {
  display: table;
  min-width: max-content;
}

@media (max-width: 767px) {
  .table-page-layout.mobile-mode .table-scroll-container {
    overflow: visible;
    border: 0;
    background: transparent;
    box-shadow: none;
  }

  .table-page-layout.mobile-mode .table-scroll-container :deep(.table-wrapper) {
    overflow: visible;
  }

  .table-page-layout.mobile-mode .table-scroll-container :deep(table) {
    min-width: 100%;
  }
}
</style>
