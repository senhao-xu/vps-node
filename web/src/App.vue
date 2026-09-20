<script setup lang="ts">
import { RouterView, useRoute } from 'vue-router'
import { computed } from 'vue'
import AppLayout from '@/components/AppLayout.vue'

const route = useRoute()
const isPublic = computed(() => route.meta.public === true)
</script>

<template>
  <AppLayout v-if="!isPublic">
    <RouterView />
  </AppLayout>
  <RouterView v-else />
</template>
