import { defineStore } from 'pinia'
import { ref } from 'vue'
import { useApi } from '../composables/useApi'

interface AgentNode {
  agent_id: string
  agent_name: string
  role: string
  parent_id: string
  zone_id: string
  endpoint: string
  status: string
}

interface Zone {
  zone_id: string
  name: string
  visibility: string
  owner_id: string
  parent_zone: string
  member_count: number
}

export const useOrchestratorStore = defineStore('orchestrator', () => {
  const api = useApi()
  const agents = ref<AgentNode[]>([])
  const zones = ref<Zone[]>([])
  const hierarchy = ref<AgentNode[]>([])
  const loading = ref(false)
  const error = ref('')

  async function fetchStatus() {
    try {
      const data = await api.get('/api/v1/orchestrator/status')
      return data
    } catch (err: any) {
      error.value = err.message
      return null
    }
  }

  async function fetchAgents() {
    loading.value = true
    try {
      agents.value = await api.get('/api/v1/orchestrator/agents')
      error.value = ''
    } catch (err: any) {
      error.value = err.message
    } finally {
      loading.value = false
    }
  }

  async function fetchHierarchy() {
    try {
      hierarchy.value = await api.get('/api/v1/orchestrator/hierarchy')
    } catch (err: any) {
      error.value = err.message
    }
  }

  async function fetchZones() {
    try {
      zones.value = await api.get('/api/v1/orchestrator/zones')
    } catch (err: any) {
      error.value = err.message
    }
  }

  async function dispatchTask(description: string, targetZone: string) {
    try {
      return await api.post('/api/v1/orchestrator/dispatch', { description, target_zone: targetZone })
    } catch (err: any) {
      error.value = err.message
      return null
    }
  }

  return {
    agents,
    zones,
    hierarchy,
    loading,
    error,
    fetchStatus,
    fetchAgents,
    fetchHierarchy,
    fetchZones,
    dispatchTask,
  }
})
