import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

interface RemoteConnection {
  id: string
  name: string
  url: string
  token: string
}

export const useRemoteStore = defineStore('remote', () => {
  const connections = ref<RemoteConnection[]>([])
  const activeId = ref<string | null>(null)
  const verifying = ref(false)
  const error = ref('')

  const activeConnection = computed(() =>
    connections.value.find((c) => c.id === activeId.value) || null,
  )

  async function loadConnections() {
    if (window.electronAPI) {
      connections.value = await window.electronAPI.remote.list()
      const active = await window.electronAPI.remote.getActive()
      activeId.value = active?.id || null
    }
  }

  async function addConnection(conn: RemoteConnection) {
    if (window.electronAPI) {
      connections.value = await window.electronAPI.remote.add(conn)
    }
  }

  async function removeConnection(id: string) {
    if (window.electronAPI) {
      connections.value = await window.electronAPI.remote.remove(id)
      if (activeId.value === id) activeId.value = null
    }
  }

  async function verifyConnection(conn: RemoteConnection) {
    verifying.value = true
    error.value = ''
    try {
      if (window.electronAPI) {
        const result = await window.electronAPI.remote.verify(conn)
        if (!result.ok) {
          error.value = result.error || 'Verification failed'
        }
        return result
      }
      return { ok: false, error: 'Electron API not available' }
    } finally {
      verifying.value = false
    }
  }

  async function setActive(id: string) {
    activeId.value = id
    if (window.electronAPI) {
      await window.electronAPI.remote.setActive(id)
    }
  }

  return {
    connections,
    activeId,
    activeConnection,
    verifying,
    error,
    loadConnections,
    addConnection,
    removeConnection,
    verifyConnection,
    setActive,
  }
})
