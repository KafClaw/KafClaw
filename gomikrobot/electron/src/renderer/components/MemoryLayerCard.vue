<script setup lang="ts">
import { computed, ref } from 'vue'
import type { MemoryLayer } from '../stores/memory'

const props = defineProps<{
  layer: MemoryLayer
  maxChunks: number
  resetting: boolean
}>()

const emit = defineEmits<{
  reset: [layerName: string]
}>()

const expanded = ref(false)

const gaugePercent = computed(() => {
  if (props.maxChunks === 0) return 0
  return Math.min(100, Math.round((props.layer.chunk_count / props.maxChunks) * 100))
})

const ttlLabel = computed(() => {
  if (props.layer.ttl_days === 0) return 'Permanent'
  return `${props.layer.ttl_days}d TTL`
})

const layerIcon = computed(() => {
  const icons: Record<string, string> = {
    soul: '\u2699',        // gear
    conversation: '\u2503', // pipe
    tool: '\u2388',         // valve
    group: '\u29C9',        // conveyor
    er1: '\u21CB',          // sync arrows
    observation: '\u2316',  // gauge
  }
  return icons[props.layer.name] || '\u25CF'
})

const infoText = computed(() => {
  const info: Record<string, string> = {
    soul: 'Soul files (SOUL.md, IDENTITY.md, etc.) define the bot\'s personality and behavior. Loaded at startup and permanently indexed. These are the foundation of who the bot is.',
    conversation: 'Every conversation Q&A pair is automatically embedded and stored. Used in RAG to recall past discussions. Expires after 30 days to keep memory fresh.',
    tool: 'When tools execute (file reads, shell commands, web searches), their outputs are indexed. Helps the bot remember what it found. Expires after 14 days.',
    group: 'Knowledge shared between bots in a collaboration group via Kafka. Includes artifacts, traces, and shared memories. Retained for 60 days.',
    er1: 'Personal memories synced from the ER1 wearable/app. Includes transcripts, locations, and tagged experiences. Permanently stored.',
    observation: 'The Observer compresses long conversations into prioritized notes. HIGH priority observations capture user preferences and decisions. Never expires.',
  }
  return info[props.layer.name] || ''
})

function handleReset() {
  emit('reset', props.layer.name)
}
</script>

<template>
  <div class="layer-card">
    <div class="accent-bar" :style="{ background: layer.color }"></div>
    <div class="card-body">
      <div class="card-header" @click="expanded = !expanded">
        <div class="layer-icon" :style="{ color: layer.color }">{{ layerIcon }}</div>
        <div class="layer-info">
          <div class="layer-name">{{ layer.name }}</div>
          <div class="layer-desc">{{ layer.description }}</div>
        </div>
        <div class="layer-badges">
          <span class="ttl-badge" :style="{ borderColor: layer.color + '66', color: layer.color }">
            {{ ttlLabel }}
          </span>
          <span class="chunk-badge">{{ layer.chunk_count.toLocaleString() }}</span>
        </div>
        <div class="expand-icon" :class="{ open: expanded }">&#9662;</div>
      </div>

      <!-- Gauge -->
      <div class="gauge-row">
        <div class="gauge-track">
          <div
            class="gauge-fill"
            :style="{ width: gaugePercent + '%', background: layer.color }"
          ></div>
          <!-- Tick marks -->
          <div class="gauge-tick" style="left: 25%"></div>
          <div class="gauge-tick" style="left: 50%"></div>
          <div class="gauge-tick" style="left: 75%"></div>
        </div>
        <span class="gauge-label">{{ gaugePercent }}%</span>
      </div>

      <!-- Expanded info -->
      <div v-if="expanded" class="card-expanded">
        <div class="info-section">
          <div class="info-title">How it works</div>
          <div class="info-text">{{ infoText }}</div>
        </div>
        <button
          class="reset-btn"
          :disabled="resetting"
          @click.stop="handleReset"
        >
          {{ resetting ? 'Resetting...' : 'Reset Layer' }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.layer-card {
  display: flex;
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 8px;
  overflow: hidden;
  transition: border-color 0.15s;
}

.layer-card:hover {
  border-color: #484f58;
}

.accent-bar {
  width: 4px;
  flex-shrink: 0;
}

.card-body {
  flex: 1;
  padding: 12px 14px;
  min-width: 0;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
}

.layer-icon {
  font-size: 18px;
  width: 28px;
  text-align: center;
  flex-shrink: 0;
}

.layer-info {
  flex: 1;
  min-width: 0;
}

.layer-name {
  font-size: 13px;
  font-weight: 600;
  color: #f0f6fc;
  text-transform: capitalize;
}

.layer-desc {
  font-size: 11px;
  color: #8b949e;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.layer-badges {
  display: flex;
  gap: 6px;
  align-items: center;
  flex-shrink: 0;
}

.ttl-badge {
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 10px;
  border: 1px solid;
}

.chunk-badge {
  font-size: 11px;
  font-weight: 600;
  color: #c9d1d9;
  font-family: monospace;
}

.expand-icon {
  font-size: 10px;
  color: #8b949e;
  transition: transform 0.15s;
  flex-shrink: 0;
}

.expand-icon.open {
  transform: rotate(180deg);
}

.gauge-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}

.gauge-track {
  flex: 1;
  height: 6px;
  background: #21262d;
  border-radius: 3px;
  position: relative;
  overflow: hidden;
}

.gauge-fill {
  height: 100%;
  border-radius: 3px;
  transition: width 0.3s ease;
}

.gauge-tick {
  position: absolute;
  top: 0;
  width: 1px;
  height: 100%;
  background: #30363d;
}

.gauge-label {
  font-size: 10px;
  color: #8b949e;
  font-family: monospace;
  width: 28px;
  text-align: right;
  flex-shrink: 0;
}

.card-expanded {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid #21262d;
}

.info-section {
  margin-bottom: 10px;
}

.info-title {
  font-size: 11px;
  font-weight: 600;
  color: #c9d1d9;
  margin-bottom: 4px;
}

.info-text {
  font-size: 11px;
  color: #8b949e;
  line-height: 1.5;
}

.reset-btn {
  font-size: 11px;
  padding: 4px 12px;
  background: #21262d;
  color: #f85149;
  border: 1px solid #f8514933;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s;
}

.reset-btn:hover:not(:disabled) {
  background: #f851491a;
  border-color: #f85149;
}

.reset-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
