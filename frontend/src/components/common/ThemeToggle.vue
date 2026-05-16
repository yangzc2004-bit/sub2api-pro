<template>
  <div
    class="theme-toggle inline-flex"
    role="group"
    :aria-label="t('nav.themeMode')"
  >
    <button
      type="button"
      class="theme-toggle-option"
      :class="{ 'theme-toggle-option-active': !isDark }"
      :aria-pressed="!isDark"
      :aria-label="t('nav.dayMode')"
      :title="t('nav.dayMode')"
      @click="setTheme('light')"
    >
      <Icon name="sun" size="sm" />
    </button>
    <button
      type="button"
      class="theme-toggle-option"
      :class="{ 'theme-toggle-option-active': isDark }"
      :aria-pressed="isDark"
      :aria-label="t('nav.nightMode')"
      :title="t('nav.nightMode')"
      @click="setTheme('dark')"
    >
      <Icon name="moon" size="sm" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useThemeMode } from '@/composables/useThemeMode'

const { t } = useI18n()
const { isDark, setTheme } = useThemeMode()
</script>

<style scoped>
.theme-toggle {
  align-items: center;
  justify-content: center;
  width: 4.5rem;
  border: 1px solid var(--app-line);
  border-radius: var(--ui-radius);
  background: var(--app-surface-2);
  padding: 0.125rem;
  gap: 0.125rem;
}

.theme-toggle-option {
  display: inline-flex;
  width: 2rem;
  height: 2rem;
  flex: 0 0 2rem;
  align-items: center;
  justify-content: center;
  border: 1px solid transparent;
  border-radius: calc(var(--ui-radius) - 1px);
  color: var(--app-muted);
  transition:
    background-color 0.16s ease,
    border-color 0.16s ease,
    color 0.16s ease;
}

.theme-toggle-option:hover {
  border-color: var(--app-line);
  background: var(--app-surface);
  color: var(--app-text);
}

.theme-toggle-option-active {
  border-color: color-mix(in srgb, var(--app-brand) 42%, var(--app-line));
  background: var(--app-surface);
  color: var(--app-brand-strong);
  box-shadow: 0 1px 2px rgb(0 0 0 / 0.08);
}
</style>
