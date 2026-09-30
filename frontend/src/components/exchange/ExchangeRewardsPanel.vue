<template>
  <section
    class="prize-center"
    data-testid="prize-center"
  >
    <div class="prize-toolbar">
      <div><h2>待领取奖品</h2><p>切换云盘账号，查看各活动的全部待领奖品。</p></div>
      <div class="prize-controls">
        <el-select
          v-model="accountID"
          filterable
          placeholder="选择云盘账号"
          :loading="accountsLoading"
          aria-label="领奖账号"
          class="prize-account"
        >
          <el-option
            v-for="account in accounts"
            :key="account.id"
            :value="account.id"
            :label="`${account.phone}${account.remark ? ' · ' + account.remark : ''}`"
          />
        </el-select>
        <el-button
          :icon="Refresh"
          :loading="loading"
          :disabled="!accountID"
          @click="loadPrizes(true)"
        >
          刷新奖品
        </el-button>
      </div>
    </div>
    <div
      v-if="accountID"
      class="prize-summary"
    >
      <span><strong>{{ data?.total ?? '—' }}</strong> 件待领取</span><span><strong>{{ data ? todayCount : '—' }}</strong> 件今日到期</span><span class="prize-updated">{{ updatedLabel }}</span>
    </div>
    <el-alert
      v-if="errorMessage"
      :title="errorMessage"
      type="error"
      :closable="false"
      show-icon
      class="prize-error"
    >
      <el-button
        link
        type="primary"
        :loading="loading"
        @click="loadPrizes(true)"
      >
        重新加载
      </el-button>
    </el-alert>
    <div
      v-if="data?.prizes.length"
      class="prize-filters"
    >
      <el-input
        v-model="keyword"
        clearable
        :prefix-icon="Search"
        placeholder="搜索奖品或活动"
        aria-label="搜索待领奖品"
      /><el-checkbox v-model="todayOnly">
        仅看今日到期
      </el-checkbox>
    </div>
    <div
      v-loading="loading"
      class="prize-body"
      :aria-busy="loading"
    >
      <el-empty
        v-if="!accountsLoading && !accounts.length"
        description="还没有云盘账号，请先在账号页面添加"
      />
      <el-empty
        v-else-if="!accountID"
        description="选择账号后查看待领奖品"
      />
      <el-skeleton
        v-else-if="loading && !data"
        :rows="6"
        animated
      />
      <el-empty
        v-else-if="data && !data.prizes.length"
        description="此账号暂无待领取奖品"
      />
      <el-empty
        v-else-if="data && !filteredPrizes.length"
        description="没有符合筛选条件的奖品"
      />
      <div
        v-else-if="data"
        class="prize-grid"
      >
        <article
          v-for="prize in pagePrizes"
          :key="prize.oId || `${prize.prizeId}-${prize.expireTime}`"
          class="prize-card"
        >
          <div class="prize-card-top">
            <div class="prize-symbol">
              <el-icon><Present /></el-icon>
            </div><el-tag
              v-if="isExpired(prize)"
              type="info"
              size="small"
            >
              已到期，状态待更新
            </el-tag><el-tag
              v-else-if="prizeExpiresToday(prize.expireTime)"
              type="warning"
              size="small"
            >
              今日到期
            </el-tag><el-tag
              v-else
              type="success"
              size="small"
            >
              待领取
            </el-tag>
          </div>
          <h3>{{ prize.prizeName || '活动奖品' }}</h3><p class="prize-activity">
            {{ prizeActivityLabel(prize.marketid, prize.marketname) }}
          </p>
          <div class="prize-expiry">
            <el-icon><Clock /></el-icon>{{ formatPrizeExpiry(prize.expireTime) }}
          </div>
          <el-button
            class="prize-guide-button"
            :disabled="isExpired(prize)"
            @click="guidePrize = prize"
          >
            领取指引<el-icon><ArrowRight /></el-icon>
          </el-button>
        </article>
      </div>
    </div>
    <el-pagination
      v-if="filteredPrizes.length > pageSize"
      v-model:current-page="page"
      :page-size="pageSize"
      :total="filteredPrizes.length"
      layout="prev, pager, next, total"
      class="prize-pagination"
    />
    <el-dialog
      :model-value="!!guidePrize"
      title="领取奖品"
      width="min(520px, calc(100vw - 32px))"
      @close="guidePrize = null"
    >
      <template v-if="guidePrize">
        <h3 class="guide-name">
          {{ guidePrize.prizeName }}
        </h3><p class="guide-expiry">
          到期时间：{{ formatPrizeExpiry(guidePrize.expireTime) }}
        </p><ol class="prize-guide">
          <li>打开所选账号登录的移动云盘 App。</li><li>进入「领奖专区」或「我的奖品」，找到此奖品。</li><li>{{ guidePrize.verifycode === 1 ? '按页面提示完成验证，并填写此奖品对应的短信验证码。' : '按页面提示确认领取。' }}</li>
        </ol><p class="guide-note">
          完成领取后，刷新此列表查看最新状态。
        </p>
      </template>
      <template #footer>
        <el-button
          type="primary"
          @click="guidePrize = null"
        >
          我知道了
        </el-button>
      </template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { isAxiosError } from 'axios'
import { ArrowRight, Clock, Present, Refresh, Search } from '@element-plus/icons-vue'
import { getAccounts, getAllAccounts, type Account } from '@/api/account'
import { getPendingPrizes, type PendingPrize, type PendingPrizeList } from '@/api/exchange/prizes'
import { formatPrizeExpiry, prizeExpiresToday, prizeExpiryMillis, prizeActivityLabel } from '@/utils/prize-display'

const props = defineProps<{ isAdmin: boolean }>()
const accounts = ref<Account[]>([])
const accountsLoading = ref(false)
const accountID = ref<number>()
const data = ref<PendingPrizeList | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const keyword = ref('')
const todayOnly = ref(false)
const page = ref(1)
const pageSize = 12
const guidePrize = ref<PendingPrize | null>(null)
let controller: AbortController | null = null
let requestSequence = 0
let disposed = false
const todayCount = computed(() => data.value?.prizes.filter(prize => prizeExpiresToday(prize.expireTime)).length ?? 0)
const updatedLabel = computed(() => data.value ? `更新于 ${new Date(data.value.fetched_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}` : '奖品状态以活动服务为准')
const filteredPrizes = computed(() => (data.value?.prizes ?? []).filter(prize => {
  const text = `${prize.prizeName} ${prize.marketname}`.toLowerCase()
  return (!keyword.value || text.includes(keyword.value.trim().toLowerCase())) && (!todayOnly.value || prizeExpiresToday(prize.expireTime))
}))
const pagePrizes = computed(() => filteredPrizes.value.slice((page.value - 1) * pageSize, page.value * pageSize))
const isExpired = (prize: PendingPrize) => { const time = prizeExpiryMillis(prize.expireTime); return time !== null && time < Date.now() }

async function loadPrizes(refresh = false) {
  if (!accountID.value || disposed) return
  const sequence = ++requestSequence
  controller?.abort()
  const currentController = new AbortController()
  controller = currentController
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await getPendingPrizes(accountID.value, refresh, currentController.signal)
    if (sequence === requestSequence && !disposed) data.value = result
  } catch (error) {
    if (sequence === requestSequence && !currentController.signal.aborted && !disposed) {
      const detail = isAxiosError<{ message?: string }>(error) ? error.response?.data?.message : undefined
      errorMessage.value = data.value
        ? `刷新失败，当前展示上次成功加载的奖品。${detail ?? ''}`
        : detail || '奖品加载失败，请检查账号授权后重试。'
    }
  } finally { if (sequence === requestSequence && !disposed) loading.value = false }
}

watch(accountID, () => { data.value = null; keyword.value = ''; todayOnly.value = false; page.value = 1; guidePrize.value = null; void loadPrizes() })
watch([keyword, todayOnly], () => { page.value = 1 })
onMounted(async () => {
  accountsLoading.value = true
  try {
    const load = props.isAdmin ? getAllAccounts : getAccounts
    const first = await load(1, 200)
    const list = [...first.accounts]
    const size = first.page_size || 200
    for (let index = 2; list.length < first.total; index++) {
      const result = await load(index, size)
      if (!result.accounts.length || disposed) break
      list.push(...result.accounts)
    }
    if (!disposed) { accounts.value = list; accountID.value = list.find(account => account.is_active)?.id ?? list[0]?.id }
  } catch { errorMessage.value = '账号列表加载失败，请重新打开领奖专区。' }
  finally { if (!disposed) accountsLoading.value = false }
})
onBeforeUnmount(() => { disposed = true; controller?.abort(); requestSequence++ })
</script>

<style scoped>
.prize-toolbar,.prize-controls,.prize-summary,.prize-filters,.prize-card-top{display:flex;align-items:center;gap:16px}.prize-toolbar{justify-content:space-between;flex-wrap:wrap;margin-bottom:20px}.prize-toolbar h2{font-size:18px;color:#182338;margin:0 0 6px}.prize-toolbar p{font-size:13px;color:#64748b;margin:0}.prize-account{width:300px}.prize-summary{padding:14px 16px;background:#f6f8fc;border-radius:10px;flex-wrap:wrap;margin-bottom:18px;font-size:13px;color:#64748b}.prize-summary strong{font-size:20px;color:#243b63;margin-right:5px}.prize-updated{margin-left:auto}.prize-filters{margin:16px 0}.prize-filters .el-input{max-width:350px}.prize-body{min-height:250px}.prize-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(235px,1fr));gap:16px}.prize-card{padding:20px;border:1px solid #e5eaf1;border-radius:12px;background:#fff;display:flex;flex-direction:column;min-width:0}.prize-card-top{justify-content:space-between;gap:8px}.prize-symbol{width:38px;height:38px;border-radius:10px;background:#eff5ff;color:#2563eb;display:grid;place-items:center;font-size:22px;flex-shrink:0}.prize-card h3{font-size:15px;color:#182338;line-height:1.6;margin:18px 0 6px;overflow-wrap:anywhere}.prize-activity{color:#64748b;font-size:12px;margin:0 0 18px;overflow-wrap:anywhere}.prize-expiry{display:flex;align-items:center;gap:6px;font-size:12px;color:#64748b;margin-top:auto;margin-bottom:16px}.prize-guide-button{width:100%;justify-content:space-between}.prize-error{margin-bottom:16px}.prize-pagination{margin-top:24px;justify-content:flex-end;flex-wrap:wrap}.prize-guide{margin:20px 0;padding-left:22px;line-height:2.2;color:#334155}.guide-name{font-size:18px;color:#182338}.guide-expiry,.guide-note{font-size:13px;color:#64748b;margin:12px 0}@media(max-width:680px){.prize-controls{width:100%;gap:8px}.prize-account{flex:1;min-width:0}.prize-updated{margin-left:0;width:100%}.prize-grid{grid-template-columns:1fr}.prize-filters{flex-wrap:wrap}.prize-filters .el-input{max-width:none}.prize-pagination{justify-content:center}}
</style>
