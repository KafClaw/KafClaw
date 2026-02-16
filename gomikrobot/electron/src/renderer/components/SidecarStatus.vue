<script setup lang="ts">
import { useModeStore } from '../stores/mode'

const modeStore = useModeStore()

function statusColor(s: string): string {
  switch (s) {
    case 'running': return '#7ee787'
    case 'starting': return '#d29922'
    case 'error': return '#f85149'
    default: return '#484f58'
  }
}
</script>

<template>
  <div class="sidecar-status">
    <span class="dot" :style="{ background: statusColor(modeStore.sidecarStatus) }"></span>
    <span class="label">{{ modeStore.sidecarStatus }}</span>
  </div>
</template>

<style scoped>
.sidecar-status {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: #8b949e;
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
</style>
