<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useModeStore } from '../stores/mode'
import { useSidecar } from '../composables/useSidecar'

const router = useRouter()
const modeStore = useModeStore()
const { status: sidecarStatus, logs } = useSidecar()

const showLogs = ref(false)

async function changeMode() {
  if (window.electronAPI) {
    await window.electronAPI.mode.set('')
  }
  modeStore.currentMode = '' as any
  router.push('/mode-picker')
}

async function restartSidecar() {
  if (window.electronAPI && modeStore.currentMode) {
    await window.electronAPI.sidecar.stop()
    await window.electronAPI.sidecar.start(modeStore.currentMode)
  }
}
</script>

<template>
  <div class="settings">
    <h2>Settings</h2>

    <div class="section">
      <h3>Operation Mode</h3>
      <p>Current: <strong>{{ modeStore.modeLabel || 'Not set' }}</strong></p>
      <button @click="changeMode">Change Mode</button>
    </div>

    <div class="section" v-if="modeStore.needsSidecar">
      <h3>Sidecar</h3>
      <p>Status: <span :class="'status-' + sidecarStatus">{{ sidecarStatus }}</span></p>
      <div class="btn-row">
        <button @click="restartSidecar">Restart</button>
        <button @click="showLogs = !showLogs">{{ showLogs ? 'Hide' : 'Show' }} Logs</button>
      </div>
      <div v-if="showLogs" class="log-viewer">
        <pre>{{ logs.join('') }}</pre>
      </div>
    </div>
  </div>
</template>

<style scoped>
.settings {
  max-width: 700px;
  margin: 0 auto;
  padding: 32px 20px;
}

h2 {
  font-size: 18px;
  color: #f0f6fc;
  margin-bottom: 24px;
}

.section {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 20px;
  margin-bottom: 16px;
}

.section h3 {
  font-size: 14px;
  color: #f0f6fc;
  margin-bottom: 8px;
}

.section p {
  font-size: 12px;
  color: #8b949e;
  margin-bottom: 12px;
}

button {
  padding: 6px 16px;
  background: #21262d;
  border: 1px solid #30363d;
  border-radius: 6px;
  color: #c9d1d9;
  font-size: 12px;
  font-family: inherit;
  cursor: pointer;
}

button:hover {
  background: #30363d;
}

.btn-row {
  display: flex;
  gap: 8px;
}

.status-running { color: #7ee787; }
.status-starting { color: #d29922; }
.status-stopped { color: #8b949e; }
.status-error { color: #f85149; }

.log-viewer {
  margin-top: 12px;
  background: #0d1117;
  border: 1px solid #30363d;
  border-radius: 6px;
  padding: 12px;
  max-height: 300px;
  overflow: auto;
}

.log-viewer pre {
  font-size: 10px;
  color: #8b949e;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
