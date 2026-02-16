<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useRemoteStore } from '../stores/remote'

const router = useRouter()
const remoteStore = useRemoteStore()

const newName = ref('')
const newUrl = ref('')
const newToken = ref('')
const verifyStatus = ref('')

onMounted(async () => {
  await remoteStore.loadConnections()
})

function generateId(): string {
  return Math.random().toString(36).substring(2, 10)
}

async function addAndVerify() {
  if (!newUrl.value.trim() || !newToken.value.trim()) return
  const conn = {
    id: generateId(),
    name: newName.value || newUrl.value,
    url: newUrl.value.replace(/\/+$/, ''),
    token: newToken.value,
  }

  verifyStatus.value = 'Verifying...'
  const result = await remoteStore.verifyConnection(conn)

  if (result.ok) {
    await remoteStore.addConnection(conn)
    verifyStatus.value = 'Connected! Navigating...'
    newName.value = ''
    newUrl.value = ''
    newToken.value = ''
    // Navigate the window to the remote Go-served timeline
    if (window.electronAPI) {
      const connectResult = await window.electronAPI.remote.connect(conn.id)
      if (!connectResult.ok) {
        verifyStatus.value = `Failed: ${connectResult.error}`
      }
      // On success the window navigates away — no further code runs here.
    }
  } else {
    verifyStatus.value = `Failed: ${result.error}`
  }
}

async function connectTo(id: string) {
  if (window.electronAPI) {
    const result = await window.electronAPI.remote.connect(id)
    if (!result.ok) {
      verifyStatus.value = `Failed: ${result.error}`
    }
    // On success the window navigates away.
  }
}

async function remove(id: string) {
  await remoteStore.removeConnection(id)
}
</script>

<template>
  <div class="remote-connect">
    <div class="connect-form">
      <h2>Connect to Remote Agent</h2>
      <p class="desc">Enter the URL and auth token of a headless GoMikroBot agent.</p>

      <label>Name (optional)</label>
      <input v-model="newName" placeholder="My Server Agent" />

      <label>URL</label>
      <input v-model="newUrl" placeholder="https://agent.example.com:18791" />

      <label>Auth Token</label>
      <input v-model="newToken" type="password" placeholder="Bearer token" />

      <button @click="addAndVerify" :disabled="!newUrl.trim() || !newToken.trim() || remoteStore.verifying">
        {{ remoteStore.verifying ? 'Verifying...' : 'Connect' }}
      </button>

      <p v-if="verifyStatus" :class="{ success: verifyStatus === 'Connected!', error: verifyStatus.startsWith('Failed') }" class="status-msg">
        {{ verifyStatus }}
      </p>
    </div>

    <div class="saved-connections" v-if="remoteStore.connections.length">
      <h3>Saved Connections</h3>
      <div
        v-for="conn in remoteStore.connections"
        :key="conn.id"
        class="conn-item"
        :class="{ active: remoteStore.activeId === conn.id }"
      >
        <div class="conn-info">
          <span class="conn-name">{{ conn.name }}</span>
          <span class="conn-url">{{ conn.url }}</span>
        </div>
        <div class="conn-actions">
          <button class="btn-connect" @click="connectTo(conn.id)">Connect</button>
          <button class="btn-remove" @click="remove(conn.id)">Remove</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.remote-connect {
  max-width: 600px;
  margin: 0 auto;
  padding: 40px 20px;
}

.connect-form {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 24px;
  margin-bottom: 24px;
}

.connect-form h2 {
  font-size: 16px;
  color: #f0f6fc;
  margin-bottom: 8px;
}

.desc {
  font-size: 12px;
  color: #8b949e;
  margin-bottom: 20px;
}

label {
  display: block;
  font-size: 11px;
  color: #8b949e;
  margin-bottom: 4px;
  margin-top: 12px;
}

input {
  width: 100%;
  padding: 8px 10px;
  background: #0d1117;
  border: 1px solid #30363d;
  border-radius: 6px;
  color: #c9d1d9;
  font-family: inherit;
  font-size: 12px;
}

input:focus {
  border-color: #58a6ff;
  outline: none;
}

button {
  margin-top: 16px;
  padding: 8px 24px;
  background: #238636;
  border: none;
  border-radius: 6px;
  color: #fff;
  font-size: 12px;
  font-family: inherit;
  cursor: pointer;
}

button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.status-msg {
  margin-top: 12px;
  font-size: 12px;
}

.success { color: #7ee787; }
.error { color: #f85149; }

.saved-connections h3 {
  font-size: 14px;
  color: #f0f6fc;
  margin-bottom: 12px;
}

.conn-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 6px;
  padding: 12px;
  margin-bottom: 8px;
}

.conn-item.active {
  border-color: #7ee787;
}

.conn-name {
  font-size: 13px;
  color: #f0f6fc;
}

.conn-url {
  font-size: 11px;
  color: #8b949e;
  display: block;
}

.conn-actions {
  display: flex;
  gap: 8px;
}

.btn-connect {
  padding: 4px 12px;
  background: #1f6feb;
  font-size: 11px;
  margin-top: 0;
}

.btn-remove {
  padding: 4px 12px;
  background: #21262d;
  font-size: 11px;
  margin-top: 0;
  color: #f85149;
}
</style>
