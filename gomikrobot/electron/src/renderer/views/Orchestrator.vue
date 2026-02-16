<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useOrchestratorStore } from '../stores/orchestrator'
import ZoneTree from '../components/ZoneTree.vue'
import HierarchyGraph from '../components/HierarchyGraph.vue'

const store = useOrchestratorStore()
const activeTab = ref<'hierarchy' | 'zones' | 'dispatch'>('hierarchy')
const dispatchDesc = ref('')
const dispatchZone = ref('')
const dispatchResult = ref('')

onMounted(async () => {
  await store.fetchAgents()
  await store.fetchHierarchy()
  await store.fetchZones()
})

async function handleDispatch() {
  if (!dispatchDesc.value.trim()) return
  const result = await store.dispatchTask(dispatchDesc.value, dispatchZone.value)
  if (result) {
    dispatchResult.value = `Task dispatched: ${result.task_id || 'OK'}`
    dispatchDesc.value = ''
  }
}
</script>

<template>
  <div class="orchestrator">
    <div class="orch-header">
      <h2>Agent Orchestrator</h2>
      <div class="tab-bar">
        <button :class="{ active: activeTab === 'hierarchy' }" @click="activeTab = 'hierarchy'">Hierarchy</button>
        <button :class="{ active: activeTab === 'zones' }" @click="activeTab = 'zones'">Zones</button>
        <button :class="{ active: activeTab === 'dispatch' }" @click="activeTab = 'dispatch'">Dispatch</button>
      </div>
    </div>

    <div class="orch-content">
      <div v-if="activeTab === 'hierarchy'" class="tab-panel">
        <HierarchyGraph :agents="store.hierarchy" />
      </div>

      <div v-if="activeTab === 'zones'" class="tab-panel">
        <ZoneTree :zones="store.zones" />
      </div>

      <div v-if="activeTab === 'dispatch'" class="tab-panel">
        <div class="dispatch-form">
          <h3>Dispatch Task</h3>
          <textarea v-model="dispatchDesc" placeholder="Task description..." rows="4"></textarea>
          <select v-model="dispatchZone">
            <option value="">All zones (public)</option>
            <option v-for="z in store.zones" :key="z.zone_id" :value="z.zone_id">
              {{ z.name }} ({{ z.visibility }})
            </option>
          </select>
          <button @click="handleDispatch" :disabled="!dispatchDesc.trim()">Dispatch</button>
          <p v-if="dispatchResult" class="result">{{ dispatchResult }}</p>
        </div>
      </div>
    </div>

    <div v-if="store.error" class="error-banner">{{ store.error }}</div>
  </div>
</template>

<style scoped>
.orchestrator {
  height: calc(100vh - 48px);
  display: flex;
  flex-direction: column;
  padding: 16px;
}

.orch-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.orch-header h2 {
  font-size: 16px;
  color: #f0f6fc;
}

.tab-bar {
  display: flex;
  gap: 4px;
}

.tab-bar button {
  padding: 6px 16px;
  background: #21262d;
  border: 1px solid #30363d;
  border-radius: 6px;
  color: #8b949e;
  font-size: 12px;
  font-family: inherit;
  cursor: pointer;
}

.tab-bar button.active {
  background: #30363d;
  color: #f0f6fc;
  border-color: #58a6ff;
}

.orch-content {
  flex: 1;
  overflow: auto;
}

.tab-panel {
  height: 100%;
}

.dispatch-form {
  max-width: 600px;
}

.dispatch-form h3 {
  font-size: 14px;
  color: #f0f6fc;
  margin-bottom: 12px;
}

.dispatch-form textarea,
.dispatch-form select {
  width: 100%;
  padding: 10px;
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 6px;
  color: #c9d1d9;
  font-family: inherit;
  font-size: 12px;
  margin-bottom: 12px;
  resize: vertical;
}

.dispatch-form button {
  padding: 8px 24px;
  background: #238636;
  border: none;
  border-radius: 6px;
  color: #fff;
  font-size: 12px;
  font-family: inherit;
  cursor: pointer;
}

.dispatch-form button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.result {
  margin-top: 12px;
  font-size: 12px;
  color: #7ee787;
}

.error-banner {
  padding: 10px;
  background: #f8514933;
  border: 1px solid #f85149;
  border-radius: 6px;
  color: #f85149;
  font-size: 12px;
  margin-top: 12px;
}
</style>
