<script setup lang="ts">
defineProps<{
  zones: Array<{
    zone_id: string
    name: string
    visibility: string
    owner_id: string
    parent_zone: string
    member_count: number
  }>
}>()

function visibilityColor(v: string): string {
  switch (v) {
    case 'private': return '#f85149'
    case 'shared': return '#d29922'
    case 'public': return '#7ee787'
    default: return '#8b949e'
  }
}
</script>

<template>
  <div class="zone-tree">
    <div v-if="zones.length === 0" class="empty">No zones configured.</div>
    <div v-for="zone in zones" :key="zone.zone_id" class="zone-node">
      <div class="zone-header">
        <span class="zone-name">{{ zone.name }}</span>
        <span class="zone-vis" :style="{ color: visibilityColor(zone.visibility) }">
          {{ zone.visibility }}
        </span>
      </div>
      <div class="zone-details">
        <span>ID: {{ zone.zone_id }}</span>
        <span>Owner: {{ zone.owner_id }}</span>
        <span>Members: {{ zone.member_count }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.zone-tree {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.empty {
  color: #8b949e;
  font-size: 13px;
  text-align: center;
  padding: 40px;
}

.zone-node {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 6px;
  padding: 12px;
}

.zone-header {
  display: flex;
  justify-content: space-between;
  margin-bottom: 6px;
}

.zone-name {
  font-size: 13px;
  color: #f0f6fc;
  font-weight: 500;
}

.zone-vis {
  font-size: 11px;
  text-transform: uppercase;
}

.zone-details {
  display: flex;
  gap: 16px;
  font-size: 10px;
  color: #8b949e;
}
</style>
