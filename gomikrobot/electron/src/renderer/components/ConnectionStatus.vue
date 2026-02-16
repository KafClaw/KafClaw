<script setup lang="ts">
import { computed } from 'vue'
import { useRemoteStore } from '../stores/remote'

const remoteStore = useRemoteStore()

const label = computed(() => {
  const conn = remoteStore.activeConnection
  if (!conn) return 'No connection'
  return conn.name
})
</script>

<template>
  <div class="connection-status">
    <span class="dot" :class="{ connected: !!remoteStore.activeConnection }"></span>
    <span class="label">{{ label }}</span>
  </div>
</template>

<style scoped>
.connection-status {
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
  background: #484f58;
}

.dot.connected {
  background: #7ee787;
}
</style>
