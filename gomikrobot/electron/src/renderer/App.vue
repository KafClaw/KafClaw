<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useModeStore } from './stores/mode'
import ConnectionStatus from './components/ConnectionStatus.vue'
import SidecarStatus from './components/SidecarStatus.vue'

const router = useRouter()
const modeStore = useModeStore()

onMounted(async () => {
  await modeStore.init()
  if (!modeStore.currentMode) {
    router.push('/mode-picker')
  }
})
</script>

<template>
  <div class="app-container">
    <header class="app-header" v-if="modeStore.currentMode">
      <div class="header-left">
        <span class="logo">GoMikroBot</span>
        <span class="mode-badge">{{ modeStore.modeLabel }}</span>
      </div>
      <nav class="header-nav">
        <router-link to="/dashboard">Dashboard</router-link>
        <router-link to="/orchestrator" v-if="modeStore.currentMode === 'full'">Orchestrator</router-link>
        <router-link to="/remote" v-if="modeStore.currentMode === 'remote'">Remote</router-link>
        <router-link to="/settings">Settings</router-link>
      </nav>
      <div class="header-right">
        <SidecarStatus v-if="modeStore.currentMode !== 'remote'" />
        <ConnectionStatus v-if="modeStore.currentMode === 'remote'" />
      </div>
    </header>
    <main class="app-main">
      <router-view />
    </main>
  </div>
</template>

<style scoped>
.app-container {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: #0d1117;
}

.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  height: 48px;
  background: #161b22;
  border-bottom: 1px solid #30363d;
  -webkit-app-region: drag;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo {
  font-size: 14px;
  font-weight: 600;
  color: #58a6ff;
}

.mode-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 12px;
  background: #1f6feb33;
  color: #58a6ff;
  text-transform: uppercase;
}

.header-nav {
  display: flex;
  gap: 4px;
  -webkit-app-region: no-drag;
}

.header-nav a {
  color: #8b949e;
  text-decoration: none;
  font-size: 12px;
  padding: 6px 12px;
  border-radius: 6px;
  transition: all 0.15s;
}

.header-nav a:hover {
  color: #c9d1d9;
  background: #21262d;
}

.header-nav a.router-link-active {
  color: #f0f6fc;
  background: #30363d;
}

.header-right {
  display: flex;
  align-items: center;
  -webkit-app-region: no-drag;
}

.app-main {
  flex: 1;
  overflow: auto;
}
</style>
