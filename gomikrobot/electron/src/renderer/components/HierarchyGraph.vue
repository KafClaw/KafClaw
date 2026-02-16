<script setup lang="ts">
import AgentCard from './AgentCard.vue'

const props = defineProps<{
  agents: Array<{
    agent_id: string
    agent_name: string
    role: string
    parent_id: string
    zone_id: string
    endpoint: string
    status: string
  }>
}>()

function getRootAgents() {
  return props.agents.filter((a) => !a.parent_id)
}

function getChildren(parentId: string) {
  return props.agents.filter((a) => a.parent_id === parentId)
}
</script>

<template>
  <div class="hierarchy-graph">
    <div v-if="agents.length === 0" class="empty">No agents in hierarchy.</div>
    <div v-else class="tree">
      <div v-for="root in getRootAgents()" :key="root.agent_id" class="tree-branch">
        <AgentCard
          :agent-id="root.agent_id"
          :agent-name="root.agent_name"
          :role="root.role"
          :status="root.status"
          :zone-id="root.zone_id"
        />
        <div class="children" v-if="getChildren(root.agent_id).length">
          <div class="connector"></div>
          <div class="child-row">
            <div v-for="child in getChildren(root.agent_id)" :key="child.agent_id" class="child-branch">
              <AgentCard
                :agent-id="child.agent_id"
                :agent-name="child.agent_name"
                :role="child.role"
                :status="child.status"
                :zone-id="child.zone_id"
              />
              <div class="grandchildren" v-if="getChildren(child.agent_id).length">
                <div class="connector"></div>
                <div class="child-row">
                  <div v-for="gc in getChildren(child.agent_id)" :key="gc.agent_id">
                    <AgentCard
                      :agent-id="gc.agent_id"
                      :agent-name="gc.agent_name"
                      :role="gc.role"
                      :status="gc.status"
                      :zone-id="gc.zone_id"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.hierarchy-graph {
  padding: 16px;
}

.empty {
  color: #8b949e;
  font-size: 13px;
  text-align: center;
  padding: 40px;
}

.tree {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
}

.tree-branch {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.connector {
  width: 2px;
  height: 20px;
  background: #30363d;
  margin: 4px auto;
}

.children, .grandchildren {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.child-row {
  display: flex;
  gap: 16px;
}

.child-branch {
  display: flex;
  flex-direction: column;
  align-items: center;
}
</style>
