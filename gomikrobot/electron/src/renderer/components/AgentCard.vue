<script setup lang="ts">
defineProps<{
  agentId: string
  agentName: string
  role: string
  status: string
  zoneId: string
}>()

function roleColor(role: string): string {
  switch (role) {
    case 'orchestrator': return '#d2a8ff'
    case 'worker': return '#7ee787'
    case 'observer': return '#8b949e'
    default: return '#c9d1d9'
  }
}
</script>

<template>
  <div class="agent-card">
    <div class="card-header">
      <span class="agent-name">{{ agentName || agentId }}</span>
      <span class="role" :style="{ color: roleColor(role) }">{{ role }}</span>
    </div>
    <div class="card-body">
      <span class="id">{{ agentId }}</span>
      <span class="zone" v-if="zoneId">Zone: {{ zoneId }}</span>
    </div>
    <div class="status-indicator" :class="status">{{ status }}</div>
  </div>
</template>

<style scoped>
.agent-card {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 12px;
  min-width: 200px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
}

.agent-name {
  font-size: 13px;
  color: #f0f6fc;
  font-weight: 500;
}

.role {
  font-size: 11px;
  text-transform: uppercase;
}

.card-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.id {
  font-size: 10px;
  color: #8b949e;
  font-family: monospace;
}

.zone {
  font-size: 10px;
  color: #58a6ff;
}

.status-indicator {
  margin-top: 8px;
  font-size: 10px;
  text-transform: uppercase;
}

.status-indicator.active { color: #7ee787; }
.status-indicator.stale { color: #d29922; }
.status-indicator.inactive { color: #484f58; }
</style>
