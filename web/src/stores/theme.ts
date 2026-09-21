import { computed, ref, watch } from 'vue'
import { defineStore } from 'pinia'

export type ThemeMode = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'vps-node-theme'

function readStoredMode(): ThemeMode {
  try {
    const value = localStorage.getItem(STORAGE_KEY)
    if (value === 'light' || value === 'dark' || value === 'system') return value
  } catch {
    return 'system'
  }
  return 'system'
}

export const useThemeStore = defineStore('theme', () => {
  const media = window.matchMedia('(prefers-color-scheme: dark)')
  const mode = ref<ThemeMode>(readStoredMode())

  const systemDark = ref(media.matches)
  media.addEventListener('change', (event) => {
    systemDark.value = event.matches
  })

  const resolved = computed<'light' | 'dark'>(() => {
    if (mode.value === 'system') return systemDark.value ? 'dark' : 'light'
    return mode.value
  })

  function apply() {
    document.documentElement.dataset.theme = resolved.value
  }

  watch(resolved, apply)

  function setMode(next: ThemeMode) {
    mode.value = next
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      return
    }
  }

  return { mode, resolved, setMode, apply }
})
