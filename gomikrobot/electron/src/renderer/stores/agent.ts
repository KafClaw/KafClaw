import { defineStore } from 'pinia'
import { ref } from 'vue'
import { useApi } from '../composables/useApi'

interface TimelineEvent {
  id: number
  event_id: string
  trace_id: string
  timestamp: string
  sender_id: string
  sender_name: string
  event_type: string
  content_text: string
  classification: string
  authorized: boolean
}

interface AgentStatus {
  version: string
  mode: string
  agent_id: string
  uptime_seconds: number
  group_enabled: boolean
  orchestrator_enabled: boolean
}

export const useAgentStore = defineStore('agent', () => {
  const api = useApi()
  const events = ref<TimelineEvent[]>([])
  const status = ref<AgentStatus | null>(null)
  const loading = ref(false)
  const error = ref('')

  async function fetchStatus() {
    try {
      status.value = await api.get('/api/v1/status')
    } catch (err: any) {
      error.value = err.message
    }
  }

  async function fetchTimeline(limit = 100, offset = 0) {
    loading.value = true
    try {
      events.value = await api.get(`/api/v1/timeline?limit=${limit}&offset=${offset}`)
      error.value = ''
    } catch (err: any) {
      error.value = err.message
    } finally {
      loading.value = false
    }
  }

  return {
    events,
    status,
    loading,
    error,
    fetchStatus,
    fetchTimeline,
  }
})
