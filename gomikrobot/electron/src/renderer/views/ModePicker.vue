<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useModeStore } from '../stores/mode'

const router = useRouter()
const modeStore = useModeStore()
const activating = ref(false)
const error = ref('')
const hoveredMode = ref('')

const modes = [
  {
    id: 'standalone' as const,
    title: 'Standalone Desktop',
    subtitle: 'LOCAL AGENT',
    description: 'Personal AI assistant. No Kafka, no group collaboration. Local-only agent loop with full dashboard.',
    color: '#a855f7',
    features: ['Local agent loop', 'Timeline & dashboard', 'WhatsApp optional', 'Offline-capable'],
  },
  {
    id: 'full' as const,
    title: 'Group Master Desktop',
    subtitle: 'MULTI-AGENT OPS',
    description: 'Full local gateway with group collaboration and multi-agent orchestration via Kafka.',
    color: '#ff6b35',
    features: ['All channels active', 'Kafka group link', 'Agent orchestrator', 'Zone management'],
  },
  {
    id: 'remote' as const,
    title: 'Remote Client',
    subtitle: 'THIN CLIENT',
    description: 'Connect to a headless GoMikroBot agent running on a server. Thin client mode.',
    color: '#22c55e',
    features: ['Remote connection', 'Bearer token auth', 'Server dashboard', 'No local binary'],
  },
]

async function selectMode(mode: 'full' | 'standalone' | 'remote') {
  if (activating.value) return
  activating.value = true
  error.value = ''

  if (mode === 'remote') {
    await modeStore.setMode(mode)
    activating.value = false
    router.push('/remote')
    return
  }

  if (window.electronAPI) {
    const result = await window.electronAPI.mode.activate(mode)
    if (!result.ok) {
      error.value = result.error || 'Failed to start gateway'
      activating.value = false
    }
  }
}
</script>

<template>
  <div class="picker-root">
    <!-- Background factory scene (SVG) -->
    <svg class="bg-scene" viewBox="0 0 1200 800" preserveAspectRatio="xMidYMid slice" xmlns="http://www.w3.org/2000/svg">
      <defs>
        <linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="#060810"/>
          <stop offset="60%" stop-color="#0a0e14"/>
          <stop offset="100%" stop-color="#0d1117"/>
        </linearGradient>
        <pattern id="grid" x="0" y="0" width="60" height="60" patternUnits="userSpaceOnUse">
          <line x1="60" y1="0" x2="60" y2="60" stroke="#1a1f2b" stroke-width="0.5" opacity="0.3"/>
          <line x1="0" y1="60" x2="60" y2="60" stroke="#1a1f2b" stroke-width="0.5" opacity="0.3"/>
        </pattern>
        <filter id="glow" x="-50%" y="-50%" width="200%" height="200%">
          <feGaussianBlur in="SourceGraphic" stdDeviation="3"/>
        </filter>
        <pattern id="conveyor" x="0" y="0" width="24" height="10" patternUnits="userSpaceOnUse">
          <rect width="24" height="10" fill="#111820"/>
          <rect x="2" y="2.5" width="8" height="5" rx="1" fill="#1a2030"/>
          <rect x="14" y="2.5" width="8" height="5" rx="1" fill="#1a2030"/>
        </pattern>
      </defs>

      <!-- Sky + grid -->
      <rect width="1200" height="800" fill="url(#sky)"/>
      <rect width="1200" height="800" fill="url(#grid)"/>

      <!-- Top-left gear -->
      <g class="gear-cluster" style="transform-origin: 90px 110px;">
        <circle cx="90" cy="110" r="50" fill="none" stroke="#ff6b35" stroke-width="1" opacity="0.08" stroke-dasharray="8 5" class="gear-ring-slow"/>
        <circle cx="90" cy="110" r="32" fill="none" stroke="#ff6b35" stroke-width="1.5" opacity="0.12" class="gear-ring-fast-rev"/>
        <circle cx="90" cy="110" r="8" fill="#ff6b35" opacity="0.1"/>
        <circle cx="90" cy="110" r="3" fill="#ff6b35" opacity="0.25"/>
      </g>

      <!-- Bottom-right gear -->
      <g>
        <circle cx="1120" cy="700" r="60" fill="none" stroke="#ff6b35" stroke-width="1" opacity="0.06" stroke-dasharray="10 6" class="gear-ring-fast-rev"/>
        <circle cx="1120" cy="700" r="38" fill="none" stroke="#ff6b35" stroke-width="1.5" opacity="0.08" class="gear-ring-slow"/>
        <circle cx="1120" cy="700" r="10" fill="#ff6b35" opacity="0.06"/>
        <circle cx="1120" cy="700" r="4" fill="#ff6b35" opacity="0.15"/>
      </g>

      <!-- Top-right small gear -->
      <circle cx="1050" cy="80" r="22" fill="none" stroke="#fbbf24" stroke-width="1" opacity="0.07" class="gear-ring-slow"/>
      <circle cx="1050" cy="80" r="3" fill="#fbbf24" opacity="0.12"/>

      <!-- Bottom-left small gear -->
      <circle cx="160" cy="720" r="18" fill="none" stroke="#a855f7" stroke-width="1" opacity="0.07" class="gear-ring-fast-rev"/>
      <circle cx="160" cy="720" r="2.5" fill="#a855f7" opacity="0.12"/>

      <!-- Horizontal pipe (upper) -->
      <rect x="50" y="218" width="1100" height="6" rx="3" fill="#ff6b35" opacity="0.035"/>
      <rect x="50" y="218" width="1100" height="6" rx="3" fill="none" stroke="#ff6b35" stroke-width="0.5" opacity="0.07"/>
      <circle cx="50" cy="221" r="6" fill="#ff6b35" opacity="0.05"/>
      <circle cx="50" cy="221" r="3.5" fill="#0a0e14"/>
      <circle cx="50" cy="221" r="1.5" fill="#ff6b35" opacity="0.2"/>
      <circle cx="1150" cy="221" r="6" fill="#ff6b35" opacity="0.05"/>
      <circle cx="1150" cy="221" r="3.5" fill="#0a0e14"/>
      <circle cx="1150" cy="221" r="1.5" fill="#ff6b35" opacity="0.2"/>
      <circle r="2" fill="#ff6b35" opacity="0.15">
        <animateMotion dur="14s" repeatCount="indefinite" path="M50,221 L1150,221"/>
      </circle>

      <!-- Horizontal pipe (lower) -->
      <rect x="80" y="580" width="1040" height="6" rx="3" fill="#ff6b35" opacity="0.035"/>
      <rect x="80" y="580" width="1040" height="6" rx="3" fill="none" stroke="#ff6b35" stroke-width="0.5" opacity="0.07"/>
      <circle cx="80" cy="583" r="6" fill="#ff6b35" opacity="0.05"/>
      <circle cx="80" cy="583" r="3.5" fill="#0a0e14"/>
      <circle cx="80" cy="583" r="1.5" fill="#ff6b35" opacity="0.2"/>
      <circle cx="1120" cy="583" r="6" fill="#ff6b35" opacity="0.05"/>
      <circle cx="1120" cy="583" r="3.5" fill="#0a0e14"/>
      <circle cx="1120" cy="583" r="1.5" fill="#ff6b35" opacity="0.2"/>
      <circle r="2" fill="#ff6b35" opacity="0.12">
        <animateMotion dur="18s" repeatCount="indefinite" begin="4s" path="M1120,583 L80,583"/>
      </circle>

      <!-- Vertical pipe (left) -->
      <rect x="42" y="130" width="4" height="540" rx="2" fill="#ff6b35" opacity="0.025"/>
      <rect x="42" y="130" width="4" height="540" rx="2" fill="none" stroke="#ff6b35" stroke-width="0.5" opacity="0.05"/>

      <!-- Vertical pipe (right) -->
      <rect x="1154" y="160" width="4" height="500" rx="2" fill="#ff6b35" opacity="0.025"/>
      <rect x="1154" y="160" width="4" height="500" rx="2" fill="none" stroke="#ff6b35" stroke-width="0.5" opacity="0.05"/>

      <!-- Steam particles -->
      <circle cx="200" cy="218" r="1.5" fill="#ff6b35" class="steam"/>
      <circle cx="600" cy="218" r="1" fill="#ff6b35" class="steam steam-d2"/>
      <circle cx="900" cy="580" r="1.5" fill="#ff6b35" class="steam steam-d3"/>
      <circle cx="400" cy="580" r="1" fill="#ff6b35" class="steam steam-d4"/>

      <!-- Conveyor belt (bottom) -->
      <rect x="0" y="786" width="1200" height="14" fill="url(#conveyor)" opacity="0.35"/>
      <line x1="0" y1="786" x2="1200" y2="786" stroke="#21262d" stroke-width="1" opacity="0.25"/>
    </svg>

    <!-- Scan line -->
    <div class="scan-overlay"></div>

    <!-- Main content -->
    <div class="picker-content">
      <!-- Logo gear -->
      <div class="logo-gear">
        <svg width="68" height="68" viewBox="0 0 68 68" xmlns="http://www.w3.org/2000/svg">
          <circle cx="34" cy="34" r="26" fill="none" stroke="#ff6b35" stroke-width="1.5" opacity="0.15" stroke-dasharray="6 4" class="gear-ring-slow"/>
          <circle cx="34" cy="34" r="18" fill="none" stroke="#ff6b35" stroke-width="2" opacity="0.25" class="gear-ring-fast-rev"/>
          <circle cx="34" cy="34" r="7" fill="#ff6b35" opacity="0.2"/>
          <circle cx="34" cy="34" r="3.5" fill="#ff6b35" opacity="0.5"/>
          <circle cx="34" cy="34" r="1.5" fill="#fcd34d" opacity="0.85"/>
          <!-- Teeth -->
          <rect v-for="t in 8" :key="t" x="32.5" y="6" width="3" height="6" rx="1" fill="#ff6b35" opacity="0.15"
            :transform="`rotate(${t * 45} 34 34)`" class="gear-ring-slow"/>
        </svg>
      </div>

      <!-- Title -->
      <div class="picker-header">
        <div class="header-label">CONTROL CENTER</div>
        <h1>GoMikroBot</h1>
        <div class="header-sub">SELECT OPERATING MODE</div>
      </div>

      <!-- Mode cards -->
      <div class="picker-cards">
        <div
          v-for="mode in modes"
          :key="mode.id"
          class="mode-card"
          :class="{ disabled: activating }"
          :style="{ '--accent': mode.color }"
          @click="selectMode(mode.id)"
          @mouseenter="hoveredMode = mode.id"
          @mouseleave="hoveredMode = ''"
        >
          <!-- Top accent pipe -->
          <div class="card-top-pipe">
            <svg width="100%" height="2" preserveAspectRatio="none">
              <rect width="100%" height="2" :fill="mode.color" :opacity="hoveredMode === mode.id ? 0.7 : 0.2"/>
            </svg>
          </div>

          <!-- Card icon (inline SVG) -->
          <div class="card-icon-wrap">
            <svg width="44" height="44" viewBox="0 0 44 44" xmlns="http://www.w3.org/2000/svg">
              <circle cx="22" cy="22" r="18" fill="none" :stroke="mode.color" stroke-width="1.5" opacity="0.15" class="icon-ring"/>
              <circle cx="22" cy="22" r="11" :fill="mode.color" opacity="0.06"/>
              <!-- Standalone: monitor -->
              <template v-if="mode.id === 'standalone'">
                <rect x="11" y="12" width="22" height="15" rx="2" fill="none" :stroke="mode.color" stroke-width="1.5" opacity="0.5"/>
                <line x1="18" y1="27" x2="18" y2="31" :stroke="mode.color" stroke-width="1" opacity="0.3"/>
                <line x1="26" y1="27" x2="26" y2="31" :stroke="mode.color" stroke-width="1" opacity="0.3"/>
                <line x1="15" y1="31" x2="29" y2="31" :stroke="mode.color" stroke-width="1.5" opacity="0.4"/>
                <circle cx="22" cy="19" r="3" :fill="mode.color" opacity="0.4"/>
              </template>
              <!-- Full: spinning gear -->
              <template v-if="mode.id === 'full'">
                <circle cx="22" cy="22" r="8" fill="none" :stroke="mode.color" stroke-width="1.5" opacity="0.35" class="gear-ring-fast-rev"/>
                <circle cx="22" cy="22" r="3.5" :fill="mode.color" opacity="0.5"/>
                <circle cx="22" cy="22" r="1.5" fill="#fcd34d" opacity="0.7"/>
                <rect v-for="t in 6" :key="t" x="21" y="12" width="2" height="3.5" rx="0.5" :fill="mode.color" opacity="0.25"
                  :transform="`rotate(${t * 60} 22 22)`" class="gear-ring-fast-rev"/>
              </template>
              <!-- Remote: signal -->
              <template v-if="mode.id === 'remote'">
                <circle cx="22" cy="24" r="3" :fill="mode.color" opacity="0.5"/>
                <path d="M15 18 A10 10 0 0 1 29 18" fill="none" :stroke="mode.color" stroke-width="1.5" opacity="0.3"/>
                <path d="M12 14 A14 14 0 0 1 32 14" fill="none" :stroke="mode.color" stroke-width="1" opacity="0.2"/>
                <line x1="22" y1="24" x2="22" y2="32" :stroke="mode.color" stroke-width="1.5" opacity="0.3"/>
                <line x1="18" y1="32" x2="26" y2="32" :stroke="mode.color" stroke-width="1.5" opacity="0.35"/>
              </template>
            </svg>
          </div>

          <!-- Title + subtitle -->
          <div class="card-title-block">
            <h2 :style="{ color: hoveredMode === mode.id ? mode.color : '#f0f6fc' }">{{ mode.title }}</h2>
            <div class="card-subtitle" :style="{ color: mode.color }">{{ mode.subtitle }}</div>
          </div>

          <!-- Decorative pipe -->
          <svg class="card-pipe" width="100%" height="16" viewBox="0 0 300 16" preserveAspectRatio="none">
            <rect x="0" y="5" width="300" height="6" rx="3" :fill="mode.color" :opacity="hoveredMode === mode.id ? 0.12 : 0.06"/>
            <rect x="0" y="5" width="300" height="6" rx="3" fill="none" :stroke="mode.color" stroke-width="0.5" :opacity="hoveredMode === mode.id ? 0.3 : 0.12"/>
            <!-- Left valve -->
            <circle cx="0" cy="8" r="5" :fill="mode.color" :opacity="hoveredMode === mode.id ? 0.15 : 0.06"/>
            <circle cx="0" cy="8" r="3" fill="#0d1117"/>
            <circle cx="0" cy="8" r="1.5" :fill="mode.color" class="valve-dot" :opacity="hoveredMode === mode.id ? 0.7 : 0.3"/>
            <!-- Right valve -->
            <circle cx="300" cy="8" r="5" :fill="mode.color" :opacity="hoveredMode === mode.id ? 0.15 : 0.06"/>
            <circle cx="300" cy="8" r="3" fill="#0d1117"/>
            <circle cx="300" cy="8" r="1.5" :fill="mode.color" class="valve-dot" :opacity="hoveredMode === mode.id ? 0.7 : 0.3"/>
            <!-- Flow bubble -->
            <circle r="2" :fill="mode.color" :opacity="hoveredMode === mode.id ? 0.5 : 0.2">
              <animateMotion dur="4s" repeatCount="indefinite" path="M0,8 L300,8"/>
            </circle>
          </svg>

          <!-- Description -->
          <p class="card-desc">{{ mode.description }}</p>

          <!-- Features -->
          <ul class="card-features">
            <li v-for="f in mode.features" :key="f" :style="{ '--dot': mode.color }">{{ f }}</li>
          </ul>

          <!-- Enter hint -->
          <div class="card-enter" :style="{ color: mode.color }">
            ENTER
            <svg width="12" height="12" viewBox="0 0 12 12"><path d="M4 2l4 4-4 4" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
          </div>
        </div>
      </div>

      <!-- Error -->
      <p v-if="error" class="error">{{ error }}</p>

      <!-- Footer -->
      <div class="picker-footer">
        <div class="footer-line"></div>
        <span>GoMikroBot</span>
        <div class="footer-line"></div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* ===== Animations ===== */
@keyframes gear-spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }
@keyframes gear-spin-rev { from { transform: rotate(360deg); } to { transform: rotate(0deg); } }
@keyframes float-up { 0% { transform: translateY(0); opacity: 0.3; } 100% { transform: translateY(-35px); opacity: 0; } }
@keyframes scan { 0% { top: -2px; } 100% { top: 100%; } }
@keyframes valve-pulse { 0%,100% { opacity: 0.3; } 50% { opacity: 0.8; } }

.gear-ring-slow { transform-origin: center; animation: gear-spin 20s linear infinite; }
.gear-ring-fast-rev { transform-origin: center; animation: gear-spin-rev 12s linear infinite; }

.steam { opacity: 0; animation: float-up 4s ease-out infinite; }
.steam-d2 { animation-delay: 1.5s; animation-duration: 5s; }
.steam-d3 { animation-delay: 0.8s; animation-duration: 4.5s; }
.steam-d4 { animation-delay: 2.5s; animation-duration: 6s; }

/* ===== Layout ===== */
.picker-root {
  position: relative;
  min-height: 100vh;
  background: #0a0e14;
  overflow: hidden;
}

.bg-scene {
  position: fixed;
  inset: 0;
  width: 100%;
  height: 100%;
  z-index: 0;
  pointer-events: none;
}

.scan-overlay {
  position: fixed;
  inset: 0;
  pointer-events: none;
  z-index: 1;
  overflow: hidden;
}
.scan-overlay::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  height: 2px;
  background: linear-gradient(90deg, transparent 20%, rgba(255,107,53,0.05) 50%, transparent 80%);
  animation: scan 7s linear infinite;
}

.picker-content {
  position: relative;
  z-index: 2;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  padding: 40px 32px;
  gap: 28px;
}

/* ===== Logo ===== */
.logo-gear {
  margin-bottom: -8px;
}

/* ===== Header ===== */
.picker-header {
  text-align: center;
}
.header-label {
  font-size: 9px;
  letter-spacing: 0.4em;
  color: #ff6b35;
  opacity: 0.45;
  margin-bottom: 6px;
}
.picker-header h1 {
  font-size: 30px;
  font-weight: 800;
  color: #f0f6fc;
  letter-spacing: 0.12em;
  margin: 0 0 6px 0;
}
.header-sub {
  font-size: 10px;
  letter-spacing: 0.3em;
  color: #4b5563;
}

/* ===== Cards ===== */
.picker-cards {
  display: flex;
  gap: 20px;
  max-width: 1020px;
  width: 100%;
}

.mode-card {
  flex: 1;
  padding: 0 24px 24px;
  background: #0d1117;
  border: 1px solid rgba(255,107,53,0.1);
  border-radius: 14px;
  cursor: pointer;
  transition: all 0.35s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
  overflow: hidden;
}
.mode-card:hover:not(.disabled) {
  border-color: var(--accent, #ff6b35);
  box-shadow: 0 0 40px color-mix(in srgb, var(--accent) 8%, transparent), 0 6px 30px rgba(0,0,0,0.3);
  transform: translateY(-5px);
}
.mode-card.disabled {
  opacity: 0.4;
  cursor: not-allowed;
  filter: grayscale(0.5);
}

.card-top-pipe {
  margin: 0 -24px 20px;
}

.card-icon-wrap {
  margin-bottom: 14px;
}
.icon-ring {
  transition: stroke-opacity 0.3s;
}
.mode-card:hover .icon-ring {
  stroke-opacity: 0.4;
}

.card-title-block {
  margin-bottom: 12px;
}
.card-title-block h2 {
  font-size: 14px;
  font-weight: 700;
  letter-spacing: 0.06em;
  margin: 0 0 3px 0;
  transition: color 0.3s;
}
.card-subtitle {
  font-size: 8px;
  letter-spacing: 0.25em;
  opacity: 0.5;
}

.card-pipe {
  display: block;
  margin: 0 -4px 14px;
  transition: opacity 0.3s;
}
.mode-card:hover .valve-dot {
  animation: valve-pulse 1.2s ease-in-out infinite;
}

.card-desc {
  font-size: 11px;
  color: #6b7280;
  line-height: 1.6;
  margin: 0 0 14px 0;
}

.card-features {
  list-style: none;
  padding: 0;
  margin: 0 0 14px 0;
}
.card-features li {
  font-size: 10px;
  color: #8b949e;
  padding: 2.5px 0;
  display: flex;
  align-items: center;
  gap: 6px;
}
.card-features li::before {
  content: '';
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--dot, #ff6b35);
  opacity: 0.4;
  flex-shrink: 0;
}

.card-enter {
  font-size: 9px;
  letter-spacing: 0.2em;
  display: flex;
  align-items: center;
  gap: 4px;
  opacity: 0;
  transition: opacity 0.3s;
}
.mode-card:hover .card-enter {
  opacity: 1;
}

/* ===== Error ===== */
.error {
  color: #f85149;
  font-size: 12px;
  background: rgba(248,81,73,0.08);
  border: 1px solid rgba(248,81,73,0.2);
  padding: 8px 16px;
  border-radius: 8px;
}

/* ===== Footer ===== */
.picker-footer {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 4px;
}
.picker-footer span {
  font-size: 9px;
  color: #30363d;
  letter-spacing: 0.3em;
  text-transform: uppercase;
}
.footer-line {
  width: 48px;
  height: 1px;
  background: linear-gradient(90deg, transparent, rgba(255,107,53,0.15), transparent);
}
</style>
