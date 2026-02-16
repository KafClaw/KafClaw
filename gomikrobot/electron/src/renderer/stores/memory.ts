import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { useApi } from '../composables/useApi'

export interface MemoryLayer {
  name: string
  source_prefix: string
  description: string
  ttl_days: number
  chunk_count: number
  color: string
}

export interface ObservationItem {
  id: string
  content: string
  priority: string
  date: string
}

export interface ExpertiseItem {
  skill: string
  score: number
  trend: string
  uses: number
}

export interface MemoryConfig {
  observer_enabled: boolean
  observer_threshold: number
  observer_max_obs: number
  er1_url: string
  er1_sync_interval_sec: number
  max_chunks: number
}

export interface MemoryStatus {
  layers: MemoryLayer[]
  working_memory: {
    entries: number
    preview: string
  }
  observer: {
    enabled: boolean
    observation_count: number
    queue_depth: number
    last_observation: string | null
  }
  observations: ObservationItem[]
  er1: {
    connected: boolean
    last_sync: string | null
    synced_count: number
    url: string
  }
  expertise: ExpertiseItem[]
  totals: {
    total_chunks: number
    max_chunks: number
    oldest?: string
    newest?: string
  }
  config: MemoryConfig
}

export const useMemoryStore = defineStore('memory', () => {
  const api = useApi()
  const status = ref<MemoryStatus | null>(null)
  const loading = ref(false)
  const error = ref('')
  const resetting = ref('')

  const totalChunks = computed(() => status.value?.totals?.total_chunks ?? 0)
  const maxChunks = computed(() => status.value?.totals?.max_chunks ?? 50000)
  const usagePercent = computed(() => {
    if (maxChunks.value === 0) return 0
    return Math.round((totalChunks.value / maxChunks.value) * 100)
  })

  async function fetchMemoryStatus() {
    loading.value = true
    try {
      status.value = await api.get('/api/v1/memory/status')
      error.value = ''
    } catch (err: any) {
      error.value = err.message
    } finally {
      loading.value = false
    }
  }

  async function resetLayer(layer: string) {
    resetting.value = layer
    try {
      await api.post('/api/v1/memory/reset', { layer })
      await fetchMemoryStatus()
      error.value = ''
    } catch (err: any) {
      error.value = err.message
    } finally {
      resetting.value = ''
    }
  }

  async function pruneNow() {
    try {
      await api.post('/api/v1/memory/prune')
      await fetchMemoryStatus()
      error.value = ''
    } catch (err: any) {
      error.value = err.message
    }
  }

  async function updateConfig(cfg: Partial<MemoryConfig>) {
    try {
      await api.post('/api/v1/memory/config', cfg)
      await fetchMemoryStatus()
      error.value = ''
    } catch (err: any) {
      error.value = err.message
    }
  }

  return {
    status,
    loading,
    error,
    resetting,
    totalChunks,
    maxChunks,
    usagePercent,
    fetchMemoryStatus,
    resetLayer,
    pruneNow,
    updateConfig,
  }
})
