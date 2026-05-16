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
  border: 1px solid rgb(230 219 201);
  background: rgb(246 242 234 / 0.82);
  padding: 0.125rem;
  gap: 0.125rem;
}

.dark .theme-toggle {
  border-color: rgb(48 56 70);
  background: rgb(17 24 39 / 0.82);
}

.theme-toggle-option {
  display: inline-flex;
  width: 2rem;
  height: 2rem;
  flex: 0 0 2rem;
  align-items: center;
  justify-content: center;
  border: 1px solid transparent;
  color: rgb(112 96 82);
  transition:
    background-color 0.16s ease,
    border-color 0.16s ease,
    color 0.16s ease;
}

.theme-toggle-option:hover {
  border-color: rgb(216 204 186);
  background: rgb(255 255 255 / 0.78);
  color: rgb(34 28 24);
}

.dark .theme-toggle-option {
  color: rgb(199 205 215);
}

.dark .theme-toggle-option:hover {
  border-color: rgb(58 67 82);
  background: rgb(31 41 55 / 0.82);
  color: rgb(255 255 255);
}

.theme-toggle-option-active {
  border-color: rgb(218 118 63 / 0.34);
  background: rgb(255 255 255);
  color: rgb(177 78 25);
  box-shadow: 0 1px 2px rgb(92 62 33 / 0.08);
}

.dark .theme-toggle-option-active {
  border-color: rgb(122 240 196 / 0.34);
  background: rgb(31 41 55);
  color: rgb(122 240 196);
  box-shadow: 0 1px 2px rgb(0 0 0 / 0.22);
}
</style>
