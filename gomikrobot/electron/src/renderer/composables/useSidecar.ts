import { ref, onMounted, onUnmounted } from 'vue'

export function useSidecar() {
  const status = ref('stopped')
  const logs = ref<string[]>([])
  let pollTimer: ReturnType<typeof setInterval> | null = null

  async function refresh() {
    if (window.electronAPI) {
      status.value = await window.electronAPI.sidecar.getStatus()
      logs.value = await window.electronAPI.sidecar.getLogs()
    }
  }

  onMounted(() => {
    if (window.electronAPI) {
      window.electronAPI.sidecar.onStatusChanged((s: string) => {
        status.value = s
      })
    }
    refresh()
    pollTimer = setInterval(refresh, 5000)
  })

  onUnmounted(() => {
    if (pollTimer) clearInterval(pollTimer)
  })

  return { status, logs, refresh }
}
