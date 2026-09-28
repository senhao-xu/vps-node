<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toDataURL } from 'qrcode'
import { getNodeShare } from '@/api/nodes'
import { errorMessage } from '@/api/http'
import { listUsers } from '@/api/users'
import type { NodeBrief, NodeShare, User } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ModalDialog from '@/components/ModalDialog.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import { copyText } from '@/utils/clipboard'
import { displayUserStatus } from '@/utils/labels'

const props = defineProps<{
  open: boolean
  node: NodeBrief | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const query = ref('')
const users = ref<User[]>([])
const loadingUsers = ref(false)
const userError = ref('')
const selectedUserId = ref<number | null>(null)
const share = ref<NodeShare | null>(null)
const loadingShare = ref(false)
const shareError = ref('')
const qrUrls = ref<string[]>([])
const copiedLink = ref('')

const selectedUser = computed(
  () => users.value.find((user) => user.id === selectedUserId.value) ?? null,
)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    query.value = ''
    users.value = []
    userError.value = ''
    selectedUserId.value = null
    share.value = null
    shareError.value = ''
    qrUrls.value = []
    copiedLink.value = ''
    void loadUsers()
  },
)

async function loadUsers() {
  loadingUsers.value = true
  userError.value = ''
  try {
    const result = await listUsers({
      query: query.value.trim() || undefined,
      page: 1,
      pageSize: 50,
    })
    users.value = result.items
  } catch (err) {
    userError.value = errorMessage(err)
  } finally {
    loadingUsers.value = false
  }
}

async function selectUser(user: User) {
  if (selectedUserId.value === user.id) return
  selectedUserId.value = user.id
  await loadShare()
}

async function loadShare() {
  const node = props.node
  const userId = selectedUserId.value
  if (!node || userId === null) return
  loadingShare.value = true
  shareError.value = ''
  share.value = null
  qrUrls.value = []
  try {
    const result = await getNodeShare(node.id, userId)
    const urls = await Promise.all(
      result.links.map((link) => toDataURL(link, { margin: 1, width: 240 }).catch(() => '')),
    )
    share.value = result
    qrUrls.value = urls
  } catch (err) {
    shareError.value = errorMessage(err)
  } finally {
    loadingShare.value = false
  }
}

async function copyLink(link: string) {
  if (!(await copyText(link))) return
  copiedLink.value = link
  setTimeout(() => {
    if (copiedLink.value === link) copiedLink.value = ''
  }, 1500)
}
</script>

<template>
  <ModalDialog
    :open="props.open"
    title="节点分享"
    :subtitle="props.node ? `${props.node.name} · ${props.node.address}:${props.node.port}` : undefined"
    :width="480"
    @close="emit('close')"
  >
    <div class="form">
      <div class="field">
        <label>选择用户</label>
        <SearchInput
          v-model="query"
          placeholder="按用户名搜索"
          @search="loadUsers"
        />
        <ErrorBanner
          :message="userError"
          @dismiss="userError = ''"
        />
        <div class="user-list">
          <span
            v-if="loadingUsers"
            class="user-empty"
          >
            <LoadingSpinner size="sm" />
          </span>
          <span
            v-else-if="users.length === 0"
            class="user-empty text-secondary"
          >
            未找到用户
          </span>
          <button
            v-for="user in users"
            :key="user.id"
            type="button"
            class="user-item"
            :class="{ selected: user.id === selectedUserId }"
            @click="selectUser(user)"
          >
            <span class="user-name mono">#{{ user.id }} {{ user.username }}</span>
            <StatusBadge v-bind="displayUserStatus(user)" />
          </button>
        </div>
      </div>

      <div
        v-if="selectedUserId !== null"
        class="share-panel"
      >
        <span
          v-if="loadingShare"
          class="share-loading"
        >
          <LoadingSpinner />
        </span>
        <ErrorBanner
          v-else-if="shareError"
          :message="shareError"
          @dismiss="shareError = ''"
        />
        <template v-else-if="share">
          <p
            v-if="!share.authorized"
            class="share-warning"
          >
            用户「{{ selectedUser?.username ?? selectedUserId }}」未授权此节点，该链接可能无法连接。
          </p>
          <div
            v-for="(link, index) in share.links"
            :key="link"
            class="link-card"
          >
            <img
              v-if="qrUrls[index]"
              class="qr-image"
              :src="qrUrls[index]"
              alt="节点分享二维码"
              width="240"
              height="240"
            >
            <span class="link-url mono">{{ link }}</span>
            <button
              type="button"
              class="btn small"
              @click="copyLink(link)"
            >
              {{ copiedLink === link ? '已复制' : '复制链接' }}
            </button>
          </div>
        </template>
      </div>
    </div>

    <template #footer>
      <button
        type="button"
        class="btn secondary"
        @click="emit('close')"
      >
        关闭
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.user-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 200px;
  overflow-y: auto;
  margin-top: var(--spacing-sm);
  padding: 4px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface-muted);
}

.user-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-md);
  font-size: var(--font-size-sm);
}

.user-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
  padding: 7px 10px;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  background: var(--color-surface);
  color: var(--color-text);
  cursor: pointer;
  text-align: left;
}

.user-item:hover {
  border-color: var(--color-border-strong);
}

.user-item.selected {
  border-color: var(--color-primary);
  background: var(--color-primary-soft);
}

.user-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.share-panel {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.share-loading {
  display: flex;
  justify-content: center;
  padding: var(--spacing-lg);
}

.share-warning {
  margin: 0;
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  background: var(--color-warning-soft);
  color: var(--color-warning);
  font-size: var(--font-size-sm);
}

.link-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.qr-image {
  display: block;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.link-url {
  max-width: 100%;
  overflow-wrap: anywhere;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  text-align: center;
}
</style>
