<script setup lang="ts">
import { onMounted, onUnmounted, ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useApi } from '../composables/useApi'

const router = useRouter()
const api = useApi()

const totalChunks = ref(0)
const maxChunks = ref(50000)
const observerEnabled = ref(false)
const er1Connected = ref(false)
const error = ref(false)

let timer: ReturnType<typeof setInterval> | null = null

async function poll() {
  try {
    const data = await api.get('/api/v1/memory/status')
    totalChunks.value = data.totals?.total_chunks ?? 0
    maxChunks.value = data.totals?.max_chunks ?? 50000
    observerEnabled.value = data.observer?.enabled ?? false
    er1Connected.value = data.er1?.connected ?? false
    error.value = false
  } catch {
    error.value = true
  }
}

onMounted(() => {
  poll()
  timer = setInterval(poll, 30000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})

const usagePercent = computed(() => {
  if (maxChunks.value === 0) return 0
  return Math.round((totalChunks.value / maxChunks.value) * 100)
})

const ledColor = computed(() => {
  if (error.value) return '#484f58'
  if (usagePercent.value > 90) return '#f85149'
  if (usagePercent.value > 70) return '#fbbf24'
  if (totalChunks.value > 0) return '#a855f7'
  return '#484f58'
})

const tooltip = computed(() => {
  if (error.value) return 'Memory: unavailable'
  const parts = [`Memory: ${totalChunks.value.toLocaleString()} chunks (${usagePercent.value}%)`]
  if (observerEnabled.value) parts.push('Observer: ON')
  if (er1Connected.value) parts.push('ER1: connected')
  return parts.join(' | ')
})

function navigate() {
  router.push('/memory')
}
</script>

<template>
  <div class="memory-led" :title="tooltip" @click="navigate">
    <span class="led-dot" :style="{ background: ledColor }"></span>
    <span class="led-label">MEM</span>
  </div>
</template>

<style scoped>
.memory-led {
  display: flex;
  align-items: center;
  gap: 5px;
  cursor: pointer;
  padding: 2px 8px;
  border-radius: 10px;
  transition: background 0.15s;
  -webkit-app-region: no-drag;
}

.memory-led:hover {
  background: #21262d;
}

.led-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  transition: background 0.3s;
  box-shadow: 0 0 4px currentColor;
}

.led-label {
  font-size: 10px;
  font-weight: 600;
  color: #8b949e;
  letter-spacing: 0.5px;
}
</style>
