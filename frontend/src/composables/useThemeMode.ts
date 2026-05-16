import { readonly, ref } from 'vue'

type ThemeMode = 'light' | 'dark'

const isDark = ref(false)
let initialized = false

function resolveInitialTheme(): ThemeMode {
  if (typeof window === 'undefined') return 'light'

  const savedTheme = window.localStorage.getItem('theme')
  if (savedTheme === 'dark' || savedTheme === 'light') {
    return savedTheme
  }

  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function applyTheme(mode: ThemeMode, persist = true): void {
  isDark.value = mode === 'dark'
  document.documentElement.classList.toggle('dark', isDark.value)
  if (persist) {
    window.localStorage.setItem('theme', mode)
  }
}

function syncFromDom(): void {
  isDark.value = document.documentElement.classList.contains('dark')
}

export function useThemeMode() {
  if (!initialized && typeof window !== 'undefined') {
    initialized = true
    const mode = resolveInitialTheme()
    applyTheme(mode, false)

    window.addEventListener('storage', (event) => {
      if (event.key === 'theme') {
        const nextMode = event.newValue === 'dark' ? 'dark' : 'light'
        applyTheme(nextMode, false)
      }
    })
  } else if (typeof document !== 'undefined') {
    syncFromDom()
  }

  function setTheme(mode: ThemeMode): void {
    applyTheme(mode, true)
  }

  function toggleTheme(): void {
    setTheme(isDark.value ? 'light' : 'dark')
  }

  return {
    isDark: readonly(isDark),
    setTheme,
    toggleTheme,
  }
}
