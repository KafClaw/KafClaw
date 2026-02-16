<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useMemoryStore } from '../stores/memory'
import MemoryPipeline from '../components/MemoryPipeline.vue'
import MemoryLayerCard from '../components/MemoryLayerCard.vue'
import WorkingMemoryPreview from '../components/WorkingMemoryPreview.vue'

const memoryStore = useMemoryStore()
const showResetAllConfirm = ref(false)
const showResetLayerConfirm = ref('')
const configDraft = ref({
  observer_threshold: 50,
  observer_max_obs: 200,
  er1_sync_interval_sec: 300,
  max_chunks: 50000,
})

let refreshTimer: ReturnType<typeof setInterval> | null = null

onMounted(async () => {
  await memoryStore.fetchMemoryStatus()
  if (memoryStore.status?.config) {
    configDraft.value = { ...configDraft.value, ...memoryStore.status.config }
  }
  refreshTimer = setInterval(() => memoryStore.fetchMemoryStatus(), 15000)
})

function confirmResetAll() {
  showResetAllConfirm.value = true
}

async function doResetAll() {
  showResetAllConfirm.value = false
  await memoryStore.resetLayer('all')
}

function confirmResetLayer(name: string) {
  showResetLayerConfirm.value = name
}

async function doResetLayer() {
  const layer = showResetLayerConfirm.value
  showResetLayerConfirm.value = ''
  await memoryStore.resetLayer(layer)
}

async function resetWorkingMemory() {
  await memoryStore.resetLayer('working_memory')
}

async function saveConfig() {
  await memoryStore.updateConfig(configDraft.value)
}

function priorityColor(p: string): string {
  switch (p) {
    case 'high': return '#f85149'
    case 'medium': return '#fbbf24'
    case 'low': return '#7ee787'
    default: return '#8b949e'
  }
}

function expertiseBarWidth(score: number): string {
  return Math.round(score * 100) + '%'
}

function expertiseColor(score: number): string {
  if (score >= 0.8) return '#7ee787'
  if (score >= 0.5) return '#58a6ff'
  if (score >= 0.3) return '#fbbf24'
  return '#8b949e'
}

function trendIcon(trend: string): string {
  switch (trend) {
    case 'improving': return '\u2191'
    case 'declining': return '\u2193'
    default: return '\u2192'
  }
}

function trendColor(trend: string): string {
  switch (trend) {
    case 'improving': return '#7ee787'
    case 'declining': return '#f85149'
    default: return '#8b949e'
  }
}

function formatDate(d: string | null): string {
  if (!d) return 'Never'
  try {
    return new Date(d).toLocaleString()
  } catch {
    return d
  }
}
</script>

<template>
  <div class="memory-page">
    <!-- Header -->
    <div class="page-header">
      <div class="page-title-group">
        <h2 class="page-title">Memory Manager</h2>
        <span class="page-subtitle" v-if="memoryStore.status">
          {{ memoryStore.totalChunks.toLocaleString() }} chunks stored
        </span>
      </div>
      <div class="header-actions">
        <button class="action-btn prune-btn" @click="memoryStore.pruneNow()">Prune Now</button>
        <button class="action-btn reset-all-btn" @click="confirmResetAll">Reset All</button>
      </div>
    </div>

    <!-- Loading / Error -->
    <div class="loading" v-if="memoryStore.loading && !memoryStore.status">Loading memory status...</div>
    <div class="error" v-if="memoryStore.error">{{ memoryStore.error }}</div>

    <template v-if="memoryStore.status">
      <!-- Pipeline (Educational) -->
      <MemoryPipeline
        :total-chunks="memoryStore.totalChunks"
        :max-chunks="memoryStore.maxChunks"
      />

      <!-- Memory Layers -->
      <div class="section-title">Memory Layers</div>
      <div class="layers-grid">
        <MemoryLayerCard
          v-for="layer in memoryStore.status.layers"
          :key="layer.name"
          :layer="layer"
          :max-chunks="memoryStore.maxChunks"
          :resetting="memoryStore.resetting === layer.name"
          @reset="confirmResetLayer"
        />
      </div>

      <!-- Bottom Grid: Working Memory + Observations -->
      <div class="bottom-grid">
        <WorkingMemoryPreview
          :entries="memoryStore.status.working_memory.entries"
          :preview="memoryStore.status.working_memory.preview"
          :resetting="memoryStore.resetting === 'working_memory'"
          @reset="resetWorkingMemory"
        />

        <!-- Observations Panel -->
        <div class="obs-card">
          <div class="obs-header">
            <div class="obs-title">Observations</div>
            <span class="obs-count">
              {{ memoryStore.status.observer.observation_count }} total
              <span v-if="memoryStore.status.observer.queue_depth > 0" class="queue-badge">
                {{ memoryStore.status.observer.queue_depth }} queued
              </span>
            </span>
          </div>
          <div class="obs-list" v-if="memoryStore.status.observations?.length">
            <div
              v-for="obs in memoryStore.status.observations"
              :key="obs.id"
              class="obs-item"
            >
              <span class="obs-priority" :style="{ color: priorityColor(obs.priority) }">
                [{{ obs.priority.toUpperCase() }}]
              </span>
              <span class="obs-content">{{ obs.content }}</span>
            </div>
          </div>
          <div class="obs-empty" v-else>
            No observations yet. The observer will compress conversations after
            {{ memoryStore.status.config.observer_threshold }} messages.
          </div>
        </div>
      </div>

      <!-- Bottom Grid: Expertise + Config -->
      <div class="bottom-grid">
        <!-- Expertise -->
        <div class="expertise-card">
          <div class="section-card-title">Expertise</div>
          <div class="expertise-list" v-if="memoryStore.status.expertise?.length">
            <div
              v-for="skill in memoryStore.status.expertise"
              :key="skill.skill"
              class="expertise-row"
            >
              <span class="skill-name">{{ skill.skill }}</span>
              <div class="skill-bar-track">
                <div
                  class="skill-bar-fill"
                  :style="{ width: expertiseBarWidth(skill.score), background: expertiseColor(skill.score) }"
                ></div>
              </div>
              <span class="skill-score">{{ Math.round(skill.score * 100) }}%</span>
              <span class="skill-trend" :style="{ color: trendColor(skill.trend) }">
                {{ trendIcon(skill.trend) }}
              </span>
              <span class="skill-uses">{{ skill.uses }}</span>
            </div>
          </div>
          <div class="expertise-empty" v-else>
            No expertise data yet. Use tools to build proficiency.
          </div>
        </div>

        <!-- Configuration -->
        <div class="config-card">
          <div class="section-card-title">Configuration</div>

          <div class="config-row">
            <span class="config-label">Observer</span>
            <span class="config-value" :class="memoryStore.status.observer.enabled ? 'on' : 'off'">
              {{ memoryStore.status.observer.enabled ? 'ON' : 'OFF' }}
            </span>
          </div>

          <div class="config-row">
            <span class="config-label">Observer threshold</span>
            <input
              type="number"
              class="config-input"
              v-model.number="configDraft.observer_threshold"
              min="10"
              max="500"
            />
          </div>

          <div class="config-row">
            <span class="config-label">Max observations</span>
            <input
              type="number"
              class="config-input"
              v-model.number="configDraft.observer_max_obs"
              min="50"
              max="1000"
            />
          </div>

          <div class="config-row">
            <span class="config-label">ER1 Status</span>
            <span class="config-value" :class="memoryStore.status.er1.connected ? 'on' : 'off'">
              {{ memoryStore.status.er1.connected ? 'Connected' : 'Disconnected' }}
            </span>
          </div>

          <div class="config-row" v-if="memoryStore.status.er1.url">
            <span class="config-label">ER1 URL</span>
            <span class="config-value mono">{{ memoryStore.status.er1.url }}</span>
          </div>

          <div class="config-row">
            <span class="config-label">ER1 last sync</span>
            <span class="config-value">{{ formatDate(memoryStore.status.er1.last_sync) }}</span>
          </div>

          <div class="config-row">
            <span class="config-label">ER1 sync interval (s)</span>
            <input
              type="number"
              class="config-input"
              v-model.number="configDraft.er1_sync_interval_sec"
              min="60"
              max="3600"
            />
          </div>

          <div class="config-row">
            <span class="config-label">Max chunks</span>
            <input
              type="number"
              class="config-input"
              v-model.number="configDraft.max_chunks"
              min="1000"
              max="200000"
              step="1000"
            />
          </div>

          <div class="config-actions">
            <button class="save-btn" @click="saveConfig">Save Config</button>
          </div>
        </div>
      </div>
    </template>

    <!-- Reset All Confirmation Modal -->
    <Teleport to="body">
      <div class="modal-overlay" v-if="showResetAllConfirm" @click.self="showResetAllConfirm = false">
        <div class="modal-box">
          <div class="modal-title">Reset All Memory?</div>
          <div class="modal-text">
            This will permanently delete all memory chunks and working memory entries.
            Soul files will need to be re-indexed on next restart.
          </div>
          <div class="modal-actions">
            <button class="modal-cancel" @click="showResetAllConfirm = false">Cancel</button>
            <button class="modal-confirm" @click="doResetAll">Reset All</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Reset Layer Confirmation Modal -->
    <Teleport to="body">
      <div class="modal-overlay" v-if="showResetLayerConfirm" @click.self="showResetLayerConfirm = ''">
        <div class="modal-box">
          <div class="modal-title">Reset {{ showResetLayerConfirm }} layer?</div>
          <div class="modal-text">
            This will permanently delete all chunks in the <strong>{{ showResetLayerConfirm }}</strong> memory layer.
          </div>
          <div class="modal-actions">
            <button class="modal-cancel" @click="showResetLayerConfirm = ''">Cancel</button>
            <button class="modal-confirm" @click="doResetLayer">Reset</button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.memory-page {
  padding: 20px;
  max-width: 1100px;
  margin: 0 auto;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.page-title {
  font-size: 18px;
  font-weight: 600;
  color: #f0f6fc;
  margin: 0;
}

.page-subtitle {
  font-size: 12px;
  color: #8b949e;
  margin-left: 12px;
}

.header-actions {
  display: flex;
  gap: 8px;
}

.action-btn {
  font-size: 12px;
  padding: 6px 14px;
  border-radius: 6px;
  cursor: pointer;
  border: 1px solid;
  transition: all 0.15s;
}

.prune-btn {
  background: #21262d;
  color: #fbbf24;
  border-color: #fbbf2433;
}

.prune-btn:hover {
  background: #fbbf241a;
  border-color: #fbbf24;
}

.reset-all-btn {
  background: #21262d;
  color: #f85149;
  border-color: #f8514933;
}

.reset-all-btn:hover {
  background: #f851491a;
  border-color: #f85149;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: #f0f6fc;
  margin-bottom: 12px;
}

.layers-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 10px;
  margin-bottom: 20px;
}

.bottom-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
  margin-bottom: 20px;
}

@media (max-width: 800px) {
  .bottom-grid {
    grid-template-columns: 1fr;
  }
  .layers-grid {
    grid-template-columns: 1fr;
  }
}

/* Observations */
.obs-card {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 14px;
}

.obs-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}

.obs-title {
  font-size: 13px;
  font-weight: 600;
  color: #f0f6fc;
}

.obs-count {
  font-size: 11px;
  color: #8b949e;
}

.queue-badge {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 8px;
  background: #fbbf241a;
  color: #fbbf24;
  margin-left: 6px;
}

.obs-list {
  max-height: 200px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.obs-item {
  font-size: 11px;
  color: #c9d1d9;
  line-height: 1.4;
  padding: 4px 0;
}

.obs-priority {
  font-weight: 600;
  font-size: 10px;
  margin-right: 4px;
  font-family: monospace;
}

.obs-content {
  color: #c9d1d9;
}

.obs-empty {
  font-size: 11px;
  color: #484f58;
  text-align: center;
  padding: 20px;
}

/* Expertise */
.expertise-card {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 14px;
}

.section-card-title {
  font-size: 13px;
  font-weight: 600;
  color: #f0f6fc;
  margin-bottom: 12px;
}

.expertise-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.expertise-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.skill-name {
  font-size: 11px;
  color: #c9d1d9;
  width: 90px;
  flex-shrink: 0;
}

.skill-bar-track {
  flex: 1;
  height: 8px;
  background: #21262d;
  border-radius: 4px;
  overflow: hidden;
}

.skill-bar-fill {
  height: 100%;
  border-radius: 4px;
  transition: width 0.3s ease;
}

.skill-score {
  font-size: 11px;
  font-family: monospace;
  color: #c9d1d9;
  width: 32px;
  text-align: right;
  flex-shrink: 0;
}

.skill-trend {
  font-size: 12px;
  width: 14px;
  text-align: center;
  flex-shrink: 0;
}

.skill-uses {
  font-size: 10px;
  color: #484f58;
  width: 30px;
  text-align: right;
  flex-shrink: 0;
}

.expertise-empty {
  font-size: 11px;
  color: #484f58;
  text-align: center;
  padding: 20px;
}

/* Configuration */
.config-card {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 14px;
}

.config-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 5px 0;
  font-size: 12px;
}

.config-label {
  color: #8b949e;
}

.config-value {
  color: #c9d1d9;
}

.config-value.on {
  color: #7ee787;
}

.config-value.off {
  color: #8b949e;
}

.config-value.mono {
  font-family: monospace;
  font-size: 10px;
}

.config-input {
  width: 80px;
  padding: 3px 8px;
  background: #0d1117;
  border: 1px solid #30363d;
  border-radius: 4px;
  color: #c9d1d9;
  font-size: 12px;
  text-align: right;
  font-family: monospace;
}

.config-input:focus {
  border-color: #58a6ff;
  outline: none;
}

.config-actions {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
}

.save-btn {
  font-size: 12px;
  padding: 5px 16px;
  background: #238636;
  color: #f0f6fc;
  border: 1px solid #2ea04366;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s;
}

.save-btn:hover {
  background: #2ea043;
}

/* Modal */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal-box {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 12px;
  padding: 24px;
  max-width: 420px;
  width: 90%;
}

.modal-title {
  font-size: 16px;
  font-weight: 600;
  color: #f0f6fc;
  margin-bottom: 10px;
}

.modal-text {
  font-size: 13px;
  color: #8b949e;
  line-height: 1.5;
  margin-bottom: 20px;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.modal-cancel {
  font-size: 12px;
  padding: 6px 14px;
  background: #21262d;
  color: #c9d1d9;
  border: 1px solid #30363d;
  border-radius: 6px;
  cursor: pointer;
}

.modal-cancel:hover {
  background: #30363d;
}

.modal-confirm {
  font-size: 12px;
  padding: 6px 14px;
  background: #da3633;
  color: #f0f6fc;
  border: 1px solid #f8514966;
  border-radius: 6px;
  cursor: pointer;
}

.modal-confirm:hover {
  background: #f85149;
}

.loading {
  text-align: center;
  color: #8b949e;
  padding: 40px;
  font-size: 13px;
}

.error {
  background: #f851491a;
  border: 1px solid #f8514933;
  color: #f85149;
  font-size: 12px;
  padding: 10px 14px;
  border-radius: 6px;
  margin-bottom: 16px;
}
</style>
