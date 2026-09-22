<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { Compass } from 'lucide-vue-next'
import EmptyState from '@/components/ui/EmptyState.vue'
</script>

<template>
  <section class="page not-found">
    <EmptyState
      :icon="Compass"
      title="页面不存在"
      hint="你访问的地址不存在或已被移除"
    >
      <template #action>
        <RouterLink
          class="btn"
          to="/"
        >
          返回仪表盘
        </RouterLink>
      </template>
    </EmptyState>
  </section>
</template>

<style scoped>
.not-found {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 60vh;
}
</style>
