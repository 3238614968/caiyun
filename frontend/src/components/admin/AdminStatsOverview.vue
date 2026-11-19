<template>
  <DashboardStatGrid
    :stats="displayStats"
    label="系统统计"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import DashboardStatGrid from '@/components/dashboard/DashboardStatGrid.vue'

export interface AdminStatItem {
  key: string
  label: string
  value: number | string
  icon: string
}

const props = defineProps<{ stats: AdminStatItem[] }>()
const displayStats = computed(() => props.stats.map(stat => ({ ...stat, diff: 0 })))
</script>
