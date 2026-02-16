<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  totalChunks: number
  maxChunks: number
}>()

const fillPercent = computed(() => {
  if (props.maxChunks === 0) return 0
  return Math.min(100, Math.round((props.totalChunks / props.maxChunks) * 100))
})
</script>

<template>
  <div class="pipeline-container">
    <div class="pipeline-title">How GoMikroBot Remembers</div>
    <div class="pipeline-svg-wrap">
      <svg viewBox="0 0 900 140" xmlns="http://www.w3.org/2000/svg" class="pipeline-svg">
        <!-- Pipe connections -->
        <line x1="170" y1="60" x2="240" y2="60" stroke="#30363d" stroke-width="4" stroke-linecap="round" />
        <line x1="370" y1="60" x2="440" y2="60" stroke="#30363d" stroke-width="4" stroke-linecap="round" />
        <line x1="570" y1="60" x2="640" y2="60" stroke="#30363d" stroke-width="4" stroke-linecap="round" />
        <line x1="770" y1="60" x2="830" y2="60" stroke="#30363d" stroke-width="4" stroke-linecap="round" />

        <!-- Flow dots (animated) -->
        <circle r="3" fill="#58a6ff" opacity="0.8">
          <animateMotion dur="2s" repeatCount="indefinite" path="M170,60 L240,60" />
        </circle>
        <circle r="3" fill="#a855f7" opacity="0.8">
          <animateMotion dur="2s" repeatCount="indefinite" path="M370,60 L440,60" begin="0.5s" />
        </circle>
        <circle r="3" fill="#22c55e" opacity="0.8">
          <animateMotion dur="2s" repeatCount="indefinite" path="M570,60 L640,60" begin="1s" />
        </circle>
        <circle r="3" fill="#fbbf24" opacity="0.8">
          <animateMotion dur="2s" repeatCount="indefinite" path="M770,60 L830,60" begin="1.5s" />
        </circle>

        <!-- Stage 1: Capture -->
        <g transform="translate(20, 20)">
          <rect x="0" y="0" width="150" height="80" rx="8" fill="#161b22" stroke="#58a6ff" stroke-width="1.5" />
          <text x="75" y="32" text-anchor="middle" fill="#58a6ff" font-size="13" font-weight="600">Capture</text>
          <text x="75" y="50" text-anchor="middle" fill="#8b949e" font-size="9">Channels</text>
          <text x="75" y="63" text-anchor="middle" fill="#8b949e" font-size="9">ER1 Sync</text>
          <text x="75" y="76" text-anchor="middle" fill="#8b949e" font-size="9">Observer</text>
        </g>

        <!-- Stage 2: Embed -->
        <g transform="translate(240, 20)">
          <rect x="0" y="0" width="130" height="80" rx="8" fill="#161b22" stroke="#a855f7" stroke-width="1.5" />
          <text x="65" y="32" text-anchor="middle" fill="#a855f7" font-size="13" font-weight="600">Embed</text>
          <text x="65" y="50" text-anchor="middle" fill="#8b949e" font-size="9">OpenAI</text>
          <text x="65" y="63" text-anchor="middle" fill="#8b949e" font-size="9">1536-dim vectors</text>
          <!-- Spinning gear -->
          <g transform="translate(108, 16)">
            <circle cx="0" cy="0" r="8" fill="none" stroke="#a855f7" stroke-width="1" opacity="0.5" />
            <animateTransform attributeName="transform" type="rotate" from="0 108 16" to="360 108 16" dur="4s" repeatCount="indefinite" additive="sum" />
            <circle cx="0" cy="-5" r="2" fill="#a855f7" opacity="0.6" />
            <circle cx="4.3" cy="2.5" r="2" fill="#a855f7" opacity="0.6" />
            <circle cx="-4.3" cy="2.5" r="2" fill="#a855f7" opacity="0.6" />
          </g>
        </g>

        <!-- Stage 3: Store -->
        <g transform="translate(440, 20)">
          <rect x="0" y="0" width="130" height="80" rx="8" fill="#161b22" stroke="#22c55e" stroke-width="1.5" />
          <text x="65" y="32" text-anchor="middle" fill="#22c55e" font-size="13" font-weight="600">Store</text>
          <text x="65" y="50" text-anchor="middle" fill="#8b949e" font-size="9">SQLite-vec</text>
          <text x="65" y="63" text-anchor="middle" fill="#8b949e" font-size="9">{{ totalChunks.toLocaleString() }} chunks</text>
          <!-- Pressure gauge -->
          <rect x="15" y="70" width="100" height="5" rx="2" fill="#21262d" />
          <rect x="15" y="70" :width="fillPercent" height="5" rx="2" :fill="fillPercent > 80 ? '#f85149' : '#22c55e'" />
        </g>

        <!-- Stage 4: Retrieve -->
        <g transform="translate(640, 20)">
          <rect x="0" y="0" width="130" height="80" rx="8" fill="#161b22" stroke="#fbbf24" stroke-width="1.5" />
          <text x="65" y="32" text-anchor="middle" fill="#fbbf24" font-size="13" font-weight="600">Retrieve</text>
          <text x="65" y="50" text-anchor="middle" fill="#8b949e" font-size="9">RAG Search</text>
          <text x="65" y="63" text-anchor="middle" fill="#8b949e" font-size="9">Cosine similarity</text>
        </g>

        <!-- Stage 5: Inject -->
        <g transform="translate(830, 20)">
          <rect x="-10" y="0" width="80" height="80" rx="8" fill="#161b22" stroke="#f0883e" stroke-width="1.5" />
          <text x="30" y="32" text-anchor="middle" fill="#f0883e" font-size="13" font-weight="600">Inject</text>
          <text x="30" y="50" text-anchor="middle" fill="#8b949e" font-size="9">System</text>
          <text x="30" y="63" text-anchor="middle" fill="#8b949e" font-size="9">Prompt</text>
        </g>

        <!-- Usage bar label -->
        <text x="505" y="130" text-anchor="middle" fill="#8b949e" font-size="9">
          {{ fillPercent }}% capacity ({{ totalChunks.toLocaleString() }} / {{ maxChunks.toLocaleString() }})
        </text>
      </svg>
    </div>
  </div>
</template>

<style scoped>
.pipeline-container {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 16px;
}

.pipeline-title {
  font-size: 13px;
  font-weight: 600;
  color: #f0f6fc;
  margin-bottom: 12px;
}

.pipeline-svg-wrap {
  overflow-x: auto;
}

.pipeline-svg {
  width: 100%;
  min-width: 600px;
  height: auto;
}
</style>
