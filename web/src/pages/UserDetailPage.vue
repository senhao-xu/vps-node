<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Activity,
  ArrowDownUp,
  CalendarClock,
  CircleGauge,
  Gauge,
  KeyRound,
  MonitorSmartphone,
  ShieldCheck,
  UserRound,
  Waypoints,
} from 'lucide-vue-next'
import { getUser, getUserNodes } from '@/api/users'
import { listNodes } from '@/api/nodes'
import { getUserCustomNodes, listCustomNodes } from '@/api/customNodes'
import { listServers } from '@/api/servers'
import { errorMessage } from '@/api/http'
import type { CustomNode, NodeBrief, Server, UserDetail } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ProgressBar from '@/components/ProgressBar.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import DetailNav, { type DetailNavItem } from '@/components/ui/DetailNav.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import MetricStrip, { type MetricStripItem } from '@/components/ui/MetricStrip.vue'
import ResourceHeader from '@/components/ui/ResourceHeader.vue'
import UserDetailBasic from '@/components/user/UserDetailBasic.vue'
import UserDetailExpiry from '@/components/user/UserDetailExpiry.vue'
import UserDetailLimits from '@/components/user/UserDetailLimits.vue'
import UserDetailTraffic from '@/components/user/UserDetailTraffic.vue'
import UserDevices from '@/components/user/UserDevices.vue'
import UserNodeAuth from '@/components/user/UserNodeAuth.vue'
import UserTrafficStats from '@/components/user/UserTrafficStats.vue'
import UserVisits from '@/components/user/UserVisits.vue'
import { formatBytes, formatDateTime, formatRemaining } from '@/utils/format'
import { displayUserStatus } from '@/utils/labels'

const route = useRoute()
const router = useRouter()

const user = ref<UserDetail | null>(null)
const nodes = ref<NodeBrief[]>([])
const servers = ref<Server[]>([])
const customNodes = ref<CustomNode[]>([])
const authorizedNodeIds = ref<number[]>([])
const authorizedCustomNodeIds = ref<number[]>([])
const loading = ref(false)
const error = ref('')

const detailNavItems: DetailNavItem[] = [
  { id: 'account', label: '账户配置', hint: '身份、流量与有效期', icon: UserRound },
  { id: 'access', label: '访问策略', hint: '设备与节点授权', icon: ShieldCheck },
  { id: 'usage', label: '用量趋势', hint: '历史流量变化', icon: CircleGauge },
  { id: 'activity', label: '访问活动', hint: '最近站点记录', icon: Activity },
]

const userMetrics = computed<MetricStripItem[]>(() => {
  const current = user.value
  if (!current) return []
  return [
    {
      key: 'traffic',
      label: '已用流量',
      value: formatBytes(current.used_bytes),
      hint: current.transfer_enable > 0 ? '额度 ' + formatBytes(current.transfer_enable) : '不限额',
      icon: ArrowDownUp,
      tone: current.used_percent >= 90 ? 'danger' : current.used_percent >= 70 ? 'warning' : 'default',
    },
    {
      key: 'remaining',
      label: '剩余流量',
      value: current.transfer_enable > 0 ? formatBytes(current.remaining_bytes) : '不限',
      icon: Gauge,
    },
    {
      key: 'devices',
      label: '在线设备',
      value: current.online_count,
      hint: current.device_limit > 0 ? '上限 ' + current.device_limit : '不限设备',
      icon: MonitorSmartphone,
    },
    {
      key: 'nodes',
      label: '授权节点',
      value: authorizedNodeIds.value.length,
      icon: Waypoints,
    },
  ]
})

const userId = computed(() => {
  const raw = route.params.id
  const id = typeof raw === 'string' ? Number(raw) : NaN
  return Number.isInteger(id) && id > 0 ? id : null
})

const accountStatus = computed(() => (user.value ? displayUserStatus(user.value) : null))
const expiryLabel = computed(() => formatRemaining(user.value?.expires_at))
const usagePercent = computed(() => {
  const value = user.value?.used_percent ?? 0
  return Math.min(100, Math.max(0, value))
})
const authorizedServerCount = computed(() => {
  const ids = new Set(authorizedNodeIds.value)
  return new Set(nodes.value.filter((node) => ids.has(node.id)).map((node) => node.server_id)).size
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
    const [detail, nodePage, serverPage, userNodes, customNodeList, userCustomNodes] =
      await Promise.all([
        getUser(id),
        listNodes({ page: 1, pageSize: 100 }),
        listServers({ page: 1, pageSize: 100 }),
        getUserNodes(id),
        listCustomNodes(),
        getUserCustomNodes(id),
      ])
    user.value = detail
    nodes.value = nodePage.items
    servers.value = serverPage.items
    customNodes.value = customNodeList.items
    authorizedNodeIds.value = userNodes.node_ids
    authorizedCustomNodeIds.value = userCustomNodes.custom_node_ids
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

function onNodesSaved(nodeIds: number[], customNodeIds: number[]) {
  authorizedNodeIds.value = nodeIds
  authorizedCustomNodeIds.value = customNodeIds
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
  <section class="page detail-page">
    <ResourceHeader
      :title="user ? user.username : '用户详情'"
      eyebrow="用户档案"
      :subtitle="user ? `UUID ${user.uuid}` : '正在读取账户资料'"
      back-to="/users"
      back-label="返回用户列表"
      :icon="UserRound"
    >
      <template #status>
        <StatusBadge
          v-if="accountStatus"
          v-bind="accountStatus"
        />
      </template>
      <template #meta>
        <span class="resource-meta-item">
          <KeyRound :size="15" />
          用户 ID <strong>#{{ user?.id ?? '—' }}</strong>
        </span>
        <span class="resource-meta-item">
          <CalendarClock :size="15" />
          创建于 <strong>{{ formatDateTime(user?.created_at) }}</strong>
        </span>
        <span class="resource-meta-item">
          <Activity :size="15" />
          最后在线 <strong>{{ formatDateTime(user?.last_online_at) }}</strong>
        </span>
      </template>
    </ResourceHeader>

    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div
      v-if="loading && !user"
      class="loading-block"
    >
      <LoadingSpinner size="lg" />
    </div>
    <template v-else-if="user">
      <MetricStrip
        :items="userMetrics"
        aria-label="用户账户指标"
      />
      <DetailNav
        :items="detailNavItems"
        aria-label="用户详情分区"
      />

      <div class="detail-workspace">
        <main class="detail-main">
          <section
            id="account"
            class="detail-section"
          >
            <div class="section-heading">
              <div class="section-heading-copy">
                <span class="section-kicker">Account</span>
                <h2>账户配置</h2>
                <p>管理身份凭证、订阅入口、流量配额与账户有效期。</p>
              </div>
            </div>
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
          </section>

          <section
            id="access"
            class="detail-section"
          >
            <div class="section-heading">
              <div class="section-heading-copy">
                <span class="section-kicker">Access</span>
                <h2>访问策略</h2>
                <p>限制并发设备和带宽，控制该用户可以连接的节点范围。</p>
              </div>
            </div>
            <div class="two-col">
              <UserDetailLimits
                :user="user"
                @updated="onUserUpdated"
              />
              <UserDevices
                :user-id="user.id"
                :nodes="nodes"
                :servers="servers"
              />
            </div>
            <UserNodeAuth
              :user-id="user.id"
              :nodes="nodes"
              :servers="servers"
              :node-ids="authorizedNodeIds"
              :custom-nodes="customNodes"
              :custom-node-ids="authorizedCustomNodeIds"
              @saved="onNodesSaved"
            />
          </section>

          <section
            id="usage"
            class="detail-section"
          >
            <div class="section-heading">
              <div class="section-heading-copy">
                <span class="section-kicker">Usage</span>
                <h2>用量趋势</h2>
                <p>按小时或天查看上行、下行及累计流量变化。</p>
              </div>
            </div>
            <UserTrafficStats :user-id="user.id" />
          </section>

          <section
            id="activity"
            class="detail-section"
          >
            <div class="section-heading">
              <div class="section-heading-copy">
                <span class="section-kicker">Activity</span>
                <h2>访问活动</h2>
                <p>核对最近访问目标、来源地址和所使用的节点。</p>
              </div>
            </div>
            <UserVisits :user-id="user.id" />
          </section>
        </main>

        <aside
          class="detail-aside"
          aria-label="用户摘要"
        >
          <div class="detail-aside-inner">
            <section class="summary-panel">
              <div class="summary-panel-head">
                <h2>账户摘要</h2>
                <CircleGauge :size="17" />
              </div>
              <dl class="summary-list">
                <div class="summary-row">
                  <dt>当前状态</dt>
                  <dd>
                    <StatusBadge
                      v-if="accountStatus"
                      v-bind="accountStatus"
                    />
                  </dd>
                </div>
                <div class="summary-row">
                  <dt>账户有效期</dt>
                  <dd>{{ expiryLabel }}</dd>
                </div>
                <div class="summary-row">
                  <dt>速度限制</dt>
                  <dd>{{ user.speed_limit > 0 ? `${user.speed_limit} Mbps` : '不限速' }}</dd>
                </div>
                <div class="summary-row">
                  <dt>设备限制</dt>
                  <dd>{{ user.device_limit > 0 ? `${user.online_count} / ${user.device_limit}` : `${user.online_count} / 不限` }}</dd>
                </div>
              </dl>
              <div class="summary-panel-footer quota-summary">
                <div class="quota-summary-line">
                  <span>流量额度</span>
                  <strong>{{ user.transfer_enable > 0 ? `${usagePercent.toFixed(1)}%` : '不限额' }}</strong>
                </div>
                <ProgressBar
                  :percent="usagePercent"
                  compact
                />
              </div>
            </section>

            <section class="summary-panel">
              <div class="summary-panel-head">
                <h2>资源关系</h2>
                <Waypoints :size="17" />
              </div>
              <dl class="summary-list">
                <div class="summary-row">
                  <dt>授权节点</dt>
                  <dd>{{ authorizedNodeIds.length }} 个</dd>
                </div>
                <div class="summary-row">
                  <dt>覆盖服务器</dt>
                  <dd>{{ authorizedServerCount }} 台</dd>
                </div>
                <div class="summary-row">
                  <dt>可选节点</dt>
                  <dd>{{ nodes.length }} 个</dd>
                </div>
              </dl>
              <div class="summary-panel-footer">
                <div class="summary-callout">
                  <ShieldCheck :size="16" />
                  <span v-if="authorizedNodeIds.length > 0">
                    授权变更保存后立即生效，并在 Agent 下次同步时应用。
                  </span>
                  <span v-else>
                    当前没有授权节点，该用户无法建立代理连接。
                  </span>
                </div>
              </div>
            </section>
          </div>
        </aside>
      </div>
    </template>
  </section>
</template>

<style scoped>
.detail-page {
  max-width: 1540px;
}

.loading-block {
  display: flex;
  justify-content: center;
  padding: 72px 0;
}

.two-col {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
  align-items: stretch;
  margin-top: 14px;
}

.two-col + :deep(.card) {
  margin-top: 14px;
}

.detail-main :deep(.card) {
  margin-top: 0;
}


.quota-summary {
  display: grid;
  gap: 8px;
}

.quota-summary-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.quota-summary-line strong {
  color: var(--color-text);
  font-weight: 700;
}

@media (max-width: 1240px) {
  .two-col {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 1024px) {
  .two-col {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 700px) {
  .two-col {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
