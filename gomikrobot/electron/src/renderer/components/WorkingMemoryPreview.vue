<script setup lang="ts">
defineProps<{
  entries: number
  preview: string
  resetting: boolean
}>()

const emit = defineEmits<{
  reset: []
}>()
</script>

<template>
  <div class="wm-card">
    <div class="wm-header">
      <div class="wm-title">Working Memory</div>
      <span class="wm-count">{{ entries }} {{ entries === 1 ? 'entry' : 'entries' }}</span>
    </div>
    <div class="wm-preview" v-if="preview">
      <pre class="wm-content">{{ preview }}</pre>
    </div>
    <div class="wm-empty" v-else>
      No working memory stored yet.
    </div>
    <div class="wm-actions">
      <button
        class="reset-btn"
        :disabled="resetting || entries === 0"
        @click="emit('reset')"
      >
        {{ resetting ? 'Resetting...' : 'Reset Working Memory' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.wm-card {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 14px;
}

.wm-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}

.wm-title {
  font-size: 13px;
  font-weight: 600;
  color: #f0f6fc;
}

.wm-count {
  font-size: 11px;
  color: #8b949e;
  font-family: monospace;
}

.wm-preview {
  background: #0d1117;
  border: 1px solid #21262d;
  border-radius: 6px;
  padding: 10px;
  max-height: 200px;
  overflow-y: auto;
}

.wm-content {
  font-size: 11px;
  color: #c9d1d9;
  white-space: pre-wrap;
  word-break: break-word;
  margin: 0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  line-height: 1.5;
}

.wm-empty {
  font-size: 11px;
  color: #484f58;
  text-align: center;
  padding: 20px;
}

.wm-actions {
  margin-top: 10px;
  display: flex;
  justify-content: flex-end;
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
