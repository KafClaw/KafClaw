<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useAgentStore } from '../stores/agent'
import { useModeStore } from '../stores/mode'

const agentStore = useAgentStore()
const modeStore = useModeStore()
const autoRefresh = ref(true)
let refreshTimer: ReturnType<typeof setInterval> | null = null

onMounted(async () => {
  await agentStore.fetchStatus()
  await agentStore.fetchTimeline()

  refreshTimer = setInterval(async () => {
    if (autoRefresh.value) {
      await agentStore.fetchTimeline()
    }
  }, 5000)
})

function formatTime(ts: string): string {
  try {
    const d = new Date(ts)
    return d.toLocaleTimeString()
  } catch {
    return ts
  }
}

function classificationColor(c: string): string {
  if (c.includes('INBOUND')) return '#58a6ff'
  if (c.includes('OUTBOUND')) return '#7ee787'
  if (c.includes('TOOL')) return '#d2a8ff'
  if (c.includes('LLM')) return '#f0883e'
  if (c.includes('POLICY')) return '#f85149'
  return '#8b949e'
}
</script>

<template>
  <div class="dashboard">
    <div class="dashboard-sidebar">
      <div class="status-card" v-if="agentStore.status">
        <h3>Agent Status</h3>
        <div class="status-row">
          <span class="label">Version</span>
          <span>{{ agentStore.status.version }}</span>
        </div>
        <div class="status-row">
          <span class="label">Mode</span>
          <span>{{ agentStore.status.mode }}</span>
        </div>
        <div class="status-row">
          <span class="label">Agent ID</span>
          <span class="mono">{{ agentStore.status.agent_id }}</span>
        </div>
        <div class="status-row">
          <span class="label">Group</span>
          <span :class="agentStore.status.group_enabled ? 'on' : 'off'">
            {{ agentStore.status.group_enabled ? 'Enabled' : 'Disabled' }}
          </span>
        </div>
        <div class="status-row">
          <span class="label">Orchestrator</span>
          <span :class="agentStore.status.orchestrator_enabled ? 'on' : 'off'">
            {{ agentStore.status.orchestrator_enabled ? 'Enabled' : 'Disabled' }}
          </span>
        </div>
      </div>

      <div class="refresh-toggle">
        <label>
          <input type="checkbox" v-model="autoRefresh" />
          Auto-refresh
        </label>
      </div>
    </div>

    <div class="timeline-panel">
      <h3>Timeline</h3>
      <div class="timeline-list" v-if="agentStore.events.length">
        <div
          v-for="event in agentStore.events"
          :key="event.event_id"
          class="timeline-event"
        >
          <div class="event-header">
            <span class="event-time">{{ formatTime(event.timestamp) }}</span>
            <span
              class="event-class"
              :style="{ color: classificationColor(event.classification) }"
            >
              {{ event.classification }}
            </span>
            <span class="event-sender">{{ event.sender_name }}</span>
          </div>
          <div class="event-content">{{ event.content_text }}</div>
        </div>
      </div>
      <div class="empty" v-else-if="!agentStore.loading">
        No events yet.
      </div>
      <div class="loading" v-else>Loading...</div>
    </div>
  </div>
</template>

<style scoped>
.dashboard {
  display: flex;
  height: calc(100vh - 48px);
}

.dashboard-sidebar {
  width: 280px;
  padding: 16px;
  border-right: 1px solid #30363d;
  overflow-y: auto;
}

.status-card {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 16px;
}

.status-card h3 {
  font-size: 13px;
  color: #f0f6fc;
  margin-bottom: 12px;
}

.status-row {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  padding: 4px 0;
}

.label { color: #8b949e; }
.mono { font-family: monospace; font-size: 10px; }
.on { color: #7ee787; }
.off { color: #8b949e; }

.refresh-toggle {
  font-size: 12px;
  color: #8b949e;
}

.refresh-toggle input { margin-right: 6px; }

.timeline-panel {
  flex: 1;
  padding: 16px;
  overflow-y: auto;
}

.timeline-panel h3 {
  font-size: 14px;
  color: #f0f6fc;
  margin-bottom: 12px;
}

.timeline-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.timeline-event {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 6px;
  padding: 10px 12px;
}

.event-header {
  display: flex;
  gap: 12px;
  margin-bottom: 6px;
  font-size: 11px;
}

.event-time { color: #8b949e; }
.event-sender { color: #58a6ff; margin-left: auto; }
.event-class { font-weight: 500; }

.event-content {
  font-size: 12px;
  color: #c9d1d9;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 120px;
  overflow: hidden;
}

.empty, .loading {
  text-align: center;
  color: #8b949e;
  padding: 40px;
  font-size: 13px;
}
</style>
