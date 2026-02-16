import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

type AppMode = 'full' | 'standalone' | 'remote' | ''

declare global {
  interface Window {
    electronAPI?: {
      mode: {
        get: () => Promise<string>
        set: (mode: string) => Promise<string>
        activate: (mode: string) => Promise<{ ok: boolean; error?: string }>
        reset: () => Promise<{ ok: boolean; error?: string }>
      }
      sidecar: {
        getStatus: () => Promise<string>
        getLogs: () => Promise<string[]>
        start: (mode: string) => Promise<string>
        stop: () => Promise<string>
        onStatusChanged: (cb: (status: string) => void) => void
      }
      config: { get: () => Promise<any>; save: (partial: Record<string, any>) => Promise<any> }
      remote: {
        list: () => Promise<any[]>
        add: (conn: any) => Promise<any[]>
        remove: (id: string) => Promise<any[]>
        verify: (conn: any) => Promise<{ ok: boolean; error?: string; data?: any }>
        setActive: (id: string) => Promise<boolean>
        getActive: () => Promise<any>
        connect: (connId: string) => Promise<{ ok: boolean; error?: string }>
      }
    }
  }
}

export const useModeStore = defineStore('mode', () => {
  const currentMode = ref<AppMode>('')
  const sidecarStatus = ref('stopped')

  const modeLabel = computed(() => {
    switch (currentMode.value) {
      case 'full': return 'Group Master Desktop'
      case 'standalone': return 'Standalone'
      case 'remote': return 'Remote'
      default: return ''
    }
  })

  const needsSidecar = computed(() => currentMode.value === 'full' || currentMode.value === 'standalone')

  async function init() {
    if (window.electronAPI) {
      const mode = await window.electronAPI.mode.get()
      currentMode.value = (mode as AppMode) || ''

      window.electronAPI.sidecar.onStatusChanged((status: string) => {
        sidecarStatus.value = status
      })

      sidecarStatus.value = await window.electronAPI.sidecar.getStatus()
    }
  }

  async function setMode(mode: AppMode) {
    currentMode.value = mode
    if (window.electronAPI) {
      await window.electronAPI.mode.set(mode)
    }
  }

  return {
    currentMode,
    sidecarStatus,
    modeLabel,
    needsSidecar,
    init,
    setMode,
  }
})
