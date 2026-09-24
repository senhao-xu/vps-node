import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { getDashboard } from '@/api/dashboard'
import { errorMessage } from '@/api/http'
import type { Dashboard } from '@/api/types'

const REFRESH_INTERVAL_MS = 30_000

export const useOverviewStore = defineStore('overview', () => {
  const stats = ref<Dashboard | null>(null)
  const loading = ref(false)
  const error = ref('')
  const lastUpdatedAt = ref<Date | null>(null)

  let inFlight: Promise<Dashboard> | null = null
  let pollTimer: ReturnType<typeof setTimeout> | null = null
  let polling = false

  const unavailable = computed(() => stats.value === null && error.value !== '')
  const stale = computed(() => stats.value !== null && error.value !== '')

  function refresh(): Promise<Dashboard> {
    if (inFlight) return inFlight

    loading.value = true
    const request = getDashboard()
      .then((result) => {
        stats.value = result
        error.value = ''
        lastUpdatedAt.value = new Date()
        return result
      })
      .catch((cause: unknown) => {
        error.value = errorMessage(cause)
        throw cause
      })
      .finally(() => {
        loading.value = false
        if (inFlight === request) inFlight = null
      })

    inFlight = request
    return request
  }

  function clearPollTimer() {
    if (pollTimer) clearTimeout(pollTimer)
    pollTimer = null
  }

  function scheduleNext() {
    clearPollTimer()
    if (!polling || document.hidden) return
    pollTimer = setTimeout(() => {
      void refresh()
        .catch(() => undefined)
        .finally(scheduleNext)
    }, REFRESH_INTERVAL_MS)
  }

  function refreshAndSchedule() {
    clearPollTimer()
    void refresh()
      .catch(() => undefined)
      .finally(scheduleNext)
  }

  function onVisibilityChange() {
    if (document.hidden) {
      clearPollTimer()
      return
    }
    refreshAndSchedule()
  }

  function startPolling() {
    if (polling) return
    polling = true
    document.addEventListener('visibilitychange', onVisibilityChange)
    if (!document.hidden) refreshAndSchedule()
  }

  function stopPolling() {
    if (!polling) return
    polling = false
    clearPollTimer()
    document.removeEventListener('visibilitychange', onVisibilityChange)
  }

  return {
    stats,
    loading,
    error,
    lastUpdatedAt,
    stale,
    unavailable,
    refresh,
    startPolling,
    stopPolling,
  }
})
