<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getUser, getUserNodes } from '@/api/users'
import { listNodes } from '@/api/nodes'
import { listServers } from '@/api/servers'
import { errorMessage } from '@/api/http'
import type { NodeBrief, Server, UserDetail } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import UserConnectionLogs from '@/components/user/UserConnectionLogs.vue'
import UserDetailBasic from '@/components/user/UserDetailBasic.vue'
import UserDetailExpiry from '@/components/user/UserDetailExpiry.vue'
import UserDetailTraffic from '@/components/user/UserDetailTraffic.vue'
import UserNodeAuth from '@/components/user/UserNodeAuth.vue'
import UserSessions from '@/components/user/UserSessions.vue'
import UserTrafficStats from '@/components/user/UserTrafficStats.vue'
import { formatDateTime } from '@/utils/format'
import { displayUserStatus } from '@/utils/labels'

const route = useRoute()
const router = useRouter()

const user = ref<UserDetail | null>(null)
const nodes = ref<NodeBrief[]>([])
const servers = ref<Server[]>([])
const authorizedNodeIds = ref<number[]>([])
const loading = ref(false)
const error = ref('')

const userId = computed(() => {
  const raw = route.params.id
  const id = typeof raw === 'string' ? Number(raw) : NaN
  return Number.isInteger(id) && id > 0 ? id : null
})

async function loadCore() {
  const id = userId.value
  if (id === null) {
    error.value = '无效的用户 ID'
    return
  }
  loading.value = true
  error.value = ''
  try {
    const [detail, nodePage, serverPage, userNodes] = await Promise.all([
      getUser(id),
      listNodes({ page: 1, pageSize: 100 }),
      listServers({ page: 1, pageSize: 100 }),
      getUserNodes(id),
    ])
    user.value = detail
    nodes.value = nodePage.items
    servers.value = serverPage.items
    authorizedNodeIds.value = userNodes.node_ids
  } catch (err) {
    error.value = errorMessage(err)
    if ((err as { status?: number }).status === 404) {
      void router.replace('/users')
    }
  } finally {
    loading.value = false
  }
}

function onUserUpdated(updated: UserDetail) {
  user.value = updated
}

function onNodesSaved(nodeIds: number[]) {
  authorizedNodeIds.value = nodeIds
  void loadCore()
}

watch(userId, () => {
  void loadCore()
})

onMounted(() => {
  void loadCore()
})
</script>

<template>
  <section class="page">
    <div class="page-header">
      <h1 class="page-title">
        <span class="eyebrow">USER PROFILE</span>
        用户详情
        <template v-if="user">
          <span class="head-status">
            <StatusBadge v-bind="displayUserStatus(user)" />
          </span>
        </template>
      </h1>
      <button
        type="button"
        class="btn secondary"
        @click="void router.push('/users')"
      >
        返回列表
      </button>
    </div>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div
      v-if="loading && !user"
      class="empty-tip"
    >
      加载中…
    </div>
    <template v-else-if="user">
      <p class="text-secondary meta-line">
        ID {{ user.id }} · 创建于 {{ formatDateTime(user.created_at) }} · 授权节点
        {{ authorizedNodeIds.length }} 个
      </p>
      <UserDetailBasic
        :user="user"
        @updated="onUserUpdated"
      />
      <div class="two-col">
        <UserDetailTraffic
          :user="user"
          @updated="onUserUpdated"
        />
        <UserDetailExpiry
          :user="user"
          @updated="onUserUpdated"
        />
      </div>
      <UserNodeAuth
        :user-id="user.id"
        :nodes="nodes"
        :servers="servers"
        :node-ids="authorizedNodeIds"
        @saved="onNodesSaved"
      />
      <UserTrafficStats :user-id="user.id" />
      <UserSessions
        :user-id="user.id"
        :nodes="nodes"
        :servers="servers"
      />
      <UserConnectionLogs
        :user-id="user.id"
        :nodes="nodes"
        :servers="servers"
      />
    </template>
  </section>
</template>

<style scoped>
.head-status {
  margin-left: var(--spacing-sm);
  vertical-align: middle;
}

.eyebrow {
  display: block;
  margin-bottom: var(--spacing-xs);
  color: var(--color-primary);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.1em;
}

.meta-line {
  margin: 0 0 var(--spacing-md);
  font-size: var(--font-size-sm);
}

.two-col {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(420px, 1fr));
  gap: var(--spacing-md);
  align-items: start;
}

.two-col .card + .card {
  margin-top: 0;
}

@media (max-width: 700px) {
  .two-col {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
