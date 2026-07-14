<template>
  <el-container
    class="layout-container"
    :class="{ 'compact-layout': compactLayout, 'reduce-motion': reduceMotion }"
  >
    <!-- 侧边栏 -->
    <el-aside
      :width="asideWidth"
      class="sidebar"
    >
      <div class="logo-container">
        <div class="logo">
          <el-icon
            :size="28"
            color="#fff"
          >
            <Cloudy />
          </el-icon>
        </div>
        <span
          v-show="!menuCollapsed"
          class="logo-text"
        >移动云盘</span>
      </div>

      <el-menu
        :default-active="activeMenu"
        :collapse="menuCollapsed"
        :collapse-transition="false"
        router
        class="sidebar-menu"
        background-color="transparent"
        text-color="#1e3a8a"
        active-text-color="#2563eb"
      >
        <el-menu-item index="/dashboard">
          <el-icon><DataLine /></el-icon>
          <template #title>
            首页
          </template>
        </el-menu-item>

        <el-menu-item index="/accounts">
          <el-icon><User /></el-icon>
          <template #title>
            账号
          </template>
        </el-menu-item>

        <el-menu-item index="/logs">
          <el-icon><List /></el-icon>
          <template #title>
            日志
          </template>
        </el-menu-item>

        <el-menu-item index="/exchange">
          <el-icon><Shop /></el-icon>
          <template #title>
            兑换
          </template>
        </el-menu-item>

        <el-menu-item
          v-if="isAdmin"
          index="/admin"
        >
          <el-icon><Setting /></el-icon>
          <template #title>
            管理
          </template>
        </el-menu-item>
      </el-menu>

      <div class="sidebar-footer">
        <el-button
          type="text"
          class="collapse-btn"
          @click="toggleCollapse"
        >
          <el-icon :size="20">
            <Fold v-if="!isCollapse" />
            <Expand v-else />
          </el-icon>
        </el-button>
      </div>
    </el-aside>

    <!-- 主内容区 -->
    <el-container class="main-container">
      <!-- 顶部导航 -->
      <el-header class="header glass-effect-light">
        <div class="header-left">
          <breadcrumb v-if="!isMobileViewport" />
          <div
            v-else
            class="mobile-page-title"
          >
            {{ currentTitle }}
          </div>
        </div>

        <div class="header-right">
          <!-- 通知中心 -->
          <NotificationCenter />

          <!-- 全屏 -->
          <el-button
            type="text"
            class="header-btn hidden-mobile-control"
            @click="toggleFullscreen"
          >
            <el-icon :size="20">
              <FullScreen />
            </el-icon>
          </el-button>

          <!-- 用户菜单 -->
          <el-dropdown
            class="user-dropdown"
            @command="handleCommand"
          >
            <div class="user-info">
              <el-avatar
                :size="isMobileViewport ? 34 : 36"
                class="user-avatar"
              >
                {{ userInitials }}
              </el-avatar>
              <div class="user-copy">
                <span class="username">{{ authStore.user?.username }}</span>
                <span class="user-role">{{ isAdmin ? '管理员' : '普通用户' }}</span>
              </div>
              <el-icon class="user-arrow">
                <ArrowDown />
              </el-icon>
            </div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="profile">
                  <el-icon><User /></el-icon>个人中心
                </el-dropdown-item>
                <el-dropdown-item command="settings">
                  <el-icon><Setting /></el-icon>系统设置
                </el-dropdown-item>
                <el-dropdown-item
                  divided
                  command="logout"
                >
                  <el-icon><SwitchButton /></el-icon>退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <!-- 内容区 -->
      <el-main class="main-content">
        <div class="page-shell">
          <router-view v-slot="{ Component }">
            <transition
              name="fade-transform"
              mode="out-in"
            >
              <component :is="Component" />
            </transition>
          </router-view>
        </div>
      </el-main>

      <!-- 页脚 -->
      <el-footer class="footer">
        <span>移动云盘管理系统 © 2026</span>
      </el-footer>
    </el-container>
  </el-container>

  <el-dialog
    v-model="profileVisible"
    :width="isMobileViewport ? 'calc(100% - 30px)' : '640px'"
    class="profile-dialog"
    destroy-on-close
  >
    <template #header>
      <div class="dialog-heading">
        <div class="dialog-heading-icon profile-heading-icon">
          <el-icon><UserFilled /></el-icon>
        </div>
        <div><strong>个人中心</strong><span>账户资料与实时服务状态</span></div>
      </div>
    </template>

    <section class="profile-hero">
      <el-avatar
        :size="64"
        class="profile-avatar"
      >
        {{ userInitials }}
      </el-avatar>
      <div class="profile-main-copy">
        <div class="profile-name-row">
          <h2>{{ authStore.user?.username || '未命名用户' }}</h2>
          <el-tag
            size="small"
            effect="light"
            type="primary"
          >
            {{ roleLabel }}
          </el-tag>
        </div>
        <p>{{ profileEmail }}</p>
        <div
          class="connection-chip"
          :class="{ online: wsClient.connected.value }"
        >
          <el-icon><CircleCheckFilled /></el-icon>{{ realtimeStatusLabel }}
        </div>
      </div>
    </section>

    <section class="profile-stat-grid">
      <div class="profile-stat-item">
        <span>用户 ID</span><strong>#{{ (authStore.user as any)?.id ?? '-' }}</strong>
      </div>
      <div class="profile-stat-item">
        <span>推送通道</span><strong>{{ realtimeTransportLabel }}</strong>
      </div>
      <div class="profile-stat-item">
        <span>账户角色</span><strong>{{ roleLabel }}</strong>
      </div>
    </section>

    <section class="profile-detail-card">
      <div class="section-title-row">
        <div><span class="section-kicker">ACCOUNT</span><h3>账户资料</h3></div>
        <el-tag
          :type="wsClient.connected.value ? 'success' : 'info'"
          effect="light"
        >
          {{ wsClient.connected.value ? '实时服务正常' : '等待连接' }}
        </el-tag>
      </div>
      <div class="profile-detail-list">
        <div><span>用户名</span><b>{{ authStore.user?.username || '-' }}</b></div>
        <div><span>邮箱</span><b>{{ profileEmail }}</b></div>
        <div><span>实时推送</span><b>{{ realtimeDescription }}</b></div>
      </div>
    </section>

    <div class="profile-quick-actions">
      <el-button
        plain
        @click="navigateFromDialog('/accounts')"
      >
        管理云盘账号<el-icon class="button-arrow">
          <ArrowRight />
        </el-icon>
      </el-button>
      <el-button
        plain
        @click="navigateFromDialog('/logs')"
      >
        查看运行日志<el-icon class="button-arrow">
          <ArrowRight />
        </el-icon>
      </el-button>
    </div>
  </el-dialog>

  <el-dialog
    v-model="settingsVisible"
    :width="isMobileViewport ? 'calc(100% - 30px)' : '640px'"
    class="settings-dialog"
    destroy-on-close
  >
    <template #header>
      <div class="dialog-heading">
        <div class="dialog-heading-icon settings-heading-icon">
          <el-icon><Setting /></el-icon>
        </div>
        <div><strong>系统设置</strong><span>界面偏好与实时连接控制</span></div>
      </div>
    </template>

    <div class="settings-summary">
      <div><span>当前设备布局</span><strong>{{ isMobileViewport ? '移动端适配模式' : compactLayout ? '紧凑布局' : '标准布局' }}</strong></div>
      <el-tag
        :type="wsClient.connected.value ? 'success' : 'info'"
        effect="light"
      >
        {{ realtimeStatusLabel }}
      </el-tag>
    </div>

    <section class="settings-section">
      <div class="settings-section-title">
        <div class="settings-section-icon">
          <el-icon><Monitor /></el-icon>
        </div>
        <div><h3>界面与布局</h3><p>这些偏好仅保存在当前浏览器。</p></div>
      </div>
      <div class="setting-row">
        <div><strong>侧边栏折叠</strong><span>为内容区域保留更多空间</span></div><el-switch
          v-model="isCollapse"
          :disabled="isMobileViewport"
        />
      </div>
      <div class="setting-row">
        <div><strong>紧凑布局</strong><span>缩小内容区边距，适合信息密集查看</span></div><el-switch v-model="compactLayout" />
      </div>
      <div class="setting-row">
        <div><strong>减少动态效果</strong><span>关闭页面切换和悬停动画</span></div><el-switch v-model="reduceMotion" />
      </div>
    </section>

    <section class="settings-section realtime-section">
      <div class="settings-section-title">
        <div class="settings-section-icon realtime-icon">
          <el-icon><Connection /></el-icon>
        </div>
        <div><h3>实时推送</h3><p>{{ realtimeDescription }}</p></div>
      </div>
      <div class="realtime-status-row">
        <div
          class="realtime-indicator"
          :class="{ online: wsClient.connected.value }"
        />
        <div><strong>{{ realtimeTransportLabel }}</strong><span>{{ wsClient.connected.value ? '连接已建立，可接收任务结果通知' : '暂未建立连接，可手动重新连接' }}</span></div>
      </div>
      <div class="settings-actions">
        <el-button
          type="primary"
          plain
          @click="reconnectPush"
        >
          <el-icon><Refresh /></el-icon>重新连接
        </el-button>
        <el-button
          :disabled="!wsClient.connected.value"
          @click="wsClient.disconnect()"
        >
          断开连接
        </el-button>
      </div>
    </section>

    <div class="settings-footer-actions">
      <el-button
        text
        type="primary"
        @click="restoreUiPreferences"
      >
        恢复默认设置
      </el-button>
      <el-button @click="settingsVisible = false">
        完成
      </el-button>
    </div>
  </el-dialog>
  <!-- 公告弹窗 -->
  <AnnouncementPopup
    v-model="popupVisible"
    :announcement="popupAnnouncement"
    @dismiss="handlePopupDismiss"
  />
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Cloudy,
  DataLine,
  User,
  List,
  Setting,
  Fold,
  Expand,
  FullScreen,
  ArrowDown,
  SwitchButton,
  UserFilled,
  CircleCheckFilled,
  Monitor,
  Connection,
  Refresh,
  ArrowRight
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'
import Breadcrumb from '@/components/Breadcrumb.vue'
import NotificationCenter from '@/components/NotificationCenter.vue'
import AnnouncementPopup from '@/components/AnnouncementPopup.vue'
import { wsClient } from '@/api/websocket'
import { getPopupAnnouncement, type Announcement } from '@/api/announcement'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const profileVisible = ref(false)
const settingsVisible = ref(false)
const popupVisible = ref(false)
const popupAnnouncement = ref<Announcement | null>(null)

const UI_PREFERENCES_STORAGE_KEY = 'caiyun_ui_preferences'

type UIPreferences = {
  isCollapse?: boolean
  compactLayout?: boolean
  reduceMotion?: boolean
}

const loadUIPreferences = (): UIPreferences => {
  if (typeof window === 'undefined') return {}
  try {
    const value = JSON.parse(localStorage.getItem(UI_PREFERENCES_STORAGE_KEY) || '{}')
    return value && typeof value === 'object' ? value as UIPreferences : {}
  } catch {
    return {}
  }
}

const initialUIPreferences = loadUIPreferences()
// 侧边栏折叠状态与界面偏好均保存在当前浏览器。
const isCollapse = ref(Boolean(initialUIPreferences.isCollapse))
const compactLayout = ref(Boolean(initialUIPreferences.compactLayout))
const reduceMotion = ref(Boolean(initialUIPreferences.reduceMotion))
const viewportWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1440)

// 当前激活的菜单
const activeMenu = computed(() => route.path)
const currentTitle = computed(() => (route.meta?.title as string) || "移动云盘")
const isMobileViewport = computed(() => viewportWidth.value <= 768)
const isTabletViewport = computed(() => viewportWidth.value <= 1280 && viewportWidth.value > 768)
const menuCollapsed = computed(() => isCollapse.value || isTabletViewport.value)
const asideWidth = computed(() => (isMobileViewport.value ? '100%' : isTabletViewport.value ? '84px' : isCollapse.value ? '64px' : '200px'))

// 是否为管理员
const isAdmin = computed(() => authStore.user?.role === 'admin')
const readAnnouncementStorageKey = computed(() => {
  const userID = authStore.user?.id || 'guest'
  return `readAnnouncements:${userID}`
})

// 用户头像文字
const userInitials = computed(() => {
  const username = authStore.user?.username || ''
  return username.charAt(0).toUpperCase()
})

const profileEmail = computed(() => authStore.user?.email || '暂未绑定邮箱')
const roleLabel = computed(() => isAdmin.value ? '管理员' : '普通用户')
const realtimeStatusLabel = computed(() => wsClient.connected.value ? '实时服务已连接' : '实时服务未连接')
const realtimeTransportLabel = computed(() => {
  switch (wsClient.transport.value) {
    case 'sse': return 'SSE 推送'
    case 'ws': return 'WebSocket'
    default: return '等待连接'
  }
})
const realtimeDescription = computed(() => {
  if (wsClient.transport.value === 'sse') return '通过 SSE 接收任务状态与兑换结果，适合 CDN 网络环境。'
  if (wsClient.transport.value === 'ws') return '通过 WebSocket 接收实时任务状态与兑换结果。'
  return '实时服务尚未建立连接，可在系统设置中手动重新连接。'
})

const persistUiPreferences = () => {
  if (typeof window === 'undefined') return
  localStorage.setItem(UI_PREFERENCES_STORAGE_KEY, JSON.stringify({
    isCollapse: isCollapse.value,
    compactLayout: compactLayout.value,
    reduceMotion: reduceMotion.value
  }))
}

watch([isCollapse, compactLayout, reduceMotion], persistUiPreferences)

const restoreUiPreferences = () => {
  isCollapse.value = false
  compactLayout.value = false
  reduceMotion.value = false
  persistUiPreferences()
  ElMessage.success('已恢复默认界面设置')
}

const reconnectPush = () => {
  wsClient.disconnect()
  window.setTimeout(() => wsClient.connect(), 0)
  ElMessage.info('正在重新建立实时连接')
}

const navigateFromDialog = (path: string) => {
  profileVisible.value = false
  settingsVisible.value = false
  void router.push(path)
}

const syncViewport = () => {
  viewportWidth.value = window.innerWidth
}
// 切换侧边栏折叠
const toggleCollapse = () => {
  if (isTabletViewport.value || isMobileViewport.value) return
  isCollapse.value = !isCollapse.value
}

// 切换全屏
const toggleFullscreen = () => {
  if (!document.fullscreenElement) {
    document.documentElement.requestFullscreen()
  } else {
    document.exitFullscreen()
  }
}

// 处理用户菜单命令
const handleCommand = async (command: string) => {
  switch (command) {
    case 'profile':
      profileVisible.value = true
      break
    case 'settings':
      settingsVisible.value = true
      break
    case 'logout':
      try {
        await ElMessageBox.confirm('确定要退出登录吗？', '提示', {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning'
        })
        wsClient.disconnect()
        await authStore.logout()
        router.push('/login')
        ElMessage.success('已退出登录')
      } catch {
        // 用户取消
      }
      break
  }
}

const loadNumberArrayFromStorage = (key: string) => {
  const rawValue = localStorage.getItem(key)
  if (!rawValue) return []

  try {
    const parsedValue = JSON.parse(rawValue)
    if (!Array.isArray(parsedValue)) {
      localStorage.removeItem(key)
      return []
    }
    return parsedValue
      .map(item => Number(item))
      .filter(item => Number.isInteger(item) && item > 0)
  } catch {
    localStorage.removeItem(key)
    return []
  }
}

const loadReadAnnouncementIDs = () => {
  const legacyDismissed = loadNumberArrayFromStorage('dismissedAnnouncements')
  const userRead = loadNumberArrayFromStorage(readAnnouncementStorageKey.value)
  return Array.from(new Set([...legacyDismissed, ...userRead]))
}

const markAnnouncementRead = (announcement: Announcement | null) => {
  if (!announcement) return
  const readIDs = loadReadAnnouncementIDs()
  if (!readIDs.includes(announcement.id)) {
    readIDs.push(announcement.id)
    localStorage.setItem(readAnnouncementStorageKey.value, JSON.stringify(readIDs))
  }
}

// 检查并显示弹窗公告
const checkPopupAnnouncement = async () => {
  try {
    const res: any = await getPopupAnnouncement()
    if (res.has_popup && res.announcements && res.announcements.length > 0) {
      const readAnnouncements = loadReadAnnouncementIDs()
      // 只自动弹出置顶且未读的弹窗公告；其他公告仅展示在首页列表中。
      const topUnreadPopup = res.announcements.find(
        (a: Announcement) => a.is_top && a.is_popup && !readAnnouncements.includes(a.id)
      )

      if (topUnreadPopup) {
        popupAnnouncement.value = topUnreadPopup
        popupVisible.value = true
      }
    }
  } catch (error) {
    // 忽略错误
  }
}

const handlePopupDismiss = () => {
  markAnnouncementRead(popupAnnouncement.value)
  popupVisible.value = false
  popupAnnouncement.value = null
}

onMounted(() => {
  syncViewport()
  window.addEventListener('resize', syncViewport)
  // 用户已登录时建立WebSocket连接
  if (authStore.isAuthenticated) {
    wsClient.connect()
  }
  // 检查弹窗公告
  checkPopupAnnouncement()
})

onUnmounted(() => {
  window.removeEventListener('resize', syncViewport)
  wsClient.disconnect()
})
</script>

<style scoped>
.layout-container {
  height: 100vh;
  background: linear-gradient(135deg, #e0f2fe 0%, #f0f9ff 50%, #e0f2fe 100%);
}

/* 侧边栏 - 浅蓝渐变毛玻璃效果 */
.sidebar {
  background: linear-gradient(180deg, rgba(224, 242, 254, 0.95) 0%, rgba(240, 249, 255, 0.9) 50%, rgba(186, 230, 253, 0.85) 100%);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  transition: width 0.3s;
  display: flex;
  flex-direction: column;
  position: relative;
  border-right: 1px solid rgba(255, 255, 255, 0.5);
  box-shadow: 4px 0 20px rgba(59, 130, 246, 0.1);
}

.logo-container {
  height: 64px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.4);
  background: rgba(255, 255, 255, 0.2);
}

.logo {
  width: 36px;
  height: 36px;
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 50%, #06b6d4 100%);
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  box-shadow: 0 4px 15px rgba(59, 130, 246, 0.35);
}

.logo-text {
  margin-left: 10px;
  font-size: 18px;
  font-weight: 600;
  color: #1e40af;
  white-space: nowrap;
  text-shadow: 0 1px 2px rgba(255, 255, 255, 0.5);
}

.sidebar-menu {
  flex: 1;
  border-right: none;
  padding: 12px 0;
  background: transparent;
}

.sidebar-menu :deep(.el-menu-item) {
  margin: 6px 10px;
  border-radius: 10px;
  height: 46px;
  line-height: 46px;
  transition: all 0.3s ease;
  font-size: 14px;
  background: rgba(255, 255, 255, 0.3);
  border: 1px solid rgba(255, 255, 255, 0.4);
}

.sidebar-menu :deep(.el-menu-item:hover) {
  background: rgba(255, 255, 255, 0.6) !important;
  color: #2563eb !important;
  transform: translateX(4px);
  box-shadow: 0 4px 12px rgba(59, 130, 246, 0.15);
}

.sidebar-menu :deep(.el-menu-item.is-active) {
  background: rgba(255, 255, 255, 0.85) !important;
  color: #2563eb !important;
  font-weight: 600;
  box-shadow: 0 4px 15px rgba(59, 130, 246, 0.2);
  border: 1px solid rgba(59, 130, 246, 0.3);
}

.sidebar-menu :deep(.el-menu-item.is-active::before) {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 24px;
  background: linear-gradient(180deg, #3b82f6 0%, #06b6d4 100%);
  border-radius: 0 3px 3px 0;
}

.sidebar-menu :deep(.el-icon) {
  font-size: 18px;
  margin-right: 10px;
  color: inherit;
}

.sidebar-footer {
  padding: 12px;
  border-top: 1px solid rgba(255, 255, 255, 0.4);
  display: flex;
  justify-content: center;
  background: rgba(255, 255, 255, 0.2);
}

.collapse-btn {
  color: #3b82f6;
  padding: 8px;
  border-radius: 8px;
  transition: all 0.25s;
}

.collapse-btn:hover {
  background: rgba(255, 255, 255, 0.6);
  color: #2563eb;
  box-shadow: 0 4px 12px rgba(59, 130, 246, 0.2);
}

/* 折叠状态下的侧边栏优化 */
.sidebar-menu :deep(.el-tooltip__trigger) {
  display: flex;
  align-items: center;
  justify-content: center;
}

.sidebar-menu :deep(.el-menu--collapse .el-menu-item) {
  margin: 6px 8px;
  padding: 0 !important;
  justify-content: center;
  background: rgba(255, 255, 255, 0.4);
}

.sidebar-menu :deep(.el-menu--collapse .el-icon) {
  margin: 0;
  font-size: 20px;
}

/* 主容器 */
.main-container {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: transparent;
}

/* 顶部导航 - 毛玻璃效果 */
.header {
  height: 70px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-bottom: 1px solid rgba(255, 255, 255, 0.6);
  box-shadow: 0 4px 20px rgba(59, 130, 246, 0.08);
}

.glass-effect-light {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
}

.header-left {
  min-width: 0;
  flex: 1;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-shrink: 0;
}

.header-btn {
  color: #3b82f6;
  padding: 10px;
  border-radius: 10px;
  transition: all 0.3s;
  background: rgba(255, 255, 255, 0.4);
  border: 1px solid rgba(255, 255, 255, 0.5);
}

.header-btn:hover {
  background: rgba(255, 255, 255, 0.8);
  color: #2563eb;
  box-shadow: 0 4px 15px rgba(59, 130, 246, 0.2);
  transform: translateY(-2px);
}

.user-dropdown {
  cursor: pointer;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 14px;
  border-radius: 12px;
  transition: all 0.3s;
  background: rgba(255, 255, 255, 0.4);
  border: 1px solid rgba(255, 255, 255, 0.5);
}

.user-info:hover {
  background: rgba(255, 255, 255, 0.8);
  box-shadow: 0 4px 15px rgba(59, 130, 246, 0.15);
}

.user-avatar {
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 50%, #06b6d4 100%);
  color: #fff;
  font-weight: 600;
  box-shadow: 0 2px 8px rgba(59, 130, 246, 0.3);
}

.username {
  font-size: 14px;
  color: #1e40af;
  font-weight: 500;
}

/* 内容区 - 透明背景 */
.main-content {
  min-width: 0;
  flex: 1;
  padding: 24px;
  overflow-y: auto;
  background: transparent;
}

/* 页脚 - 毛玻璃效果 */
.footer {
  height: 50px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, 0.6);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  border-top: 1px solid rgba(255, 255, 255, 0.5);
  color: #3b82f6;
  font-size: 13px;
}

/* 页面过渡动画 */
.fade-transform-leave-active,
.fade-transform-enter-active {
  transition: all 0.3s;
}

.fade-transform-enter-from {
  opacity: 0;
  transform: translateX(-20px);
}

.fade-transform-leave-to {
  opacity: 0;
  transform: translateX(20px);
}

/* 滚动条样式 */
.main-content::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

.main-content::-webkit-scrollbar-thumb {
  background: #c0c4cc;
  border-radius: 3px;
}

.main-content::-webkit-scrollbar-track {
  background: transparent;
}

/* 个人中心与系统设置 */
:deep(.profile-dialog),
:deep(.settings-dialog) {
  border-radius: 22px;
  overflow: hidden;
  box-shadow: 0 24px 70px rgba(30, 64, 175, 0.24);
}

:deep(.profile-dialog .el-dialog__header),
:deep(.settings-dialog .el-dialog__header) {
  margin: 0;
  padding: 24px 28px 16px;
  border-bottom: 1px solid #edf2f7;
}

:deep(.profile-dialog .el-dialog__body),
:deep(.settings-dialog .el-dialog__body) {
  padding: 22px 28px 26px;
}

.dialog-heading,
.settings-section-title,
.section-title-row,
.profile-name-row,
.settings-summary,
.realtime-status-row,
.setting-row,
.profile-quick-actions,
.settings-footer-actions {
  display: flex;
  align-items: center;
}

.dialog-heading { gap: 12px; }
.dialog-heading-icon,
.settings-section-icon {
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  color: #fff;
  background: linear-gradient(135deg, #3b82f6, #06b6d4);
  box-shadow: 0 8px 18px rgba(37, 99, 235, 0.2);
}
.dialog-heading-icon { width: 38px; height: 38px; border-radius: 12px; font-size: 19px; }
.settings-heading-icon { background: linear-gradient(135deg, #6366f1, #8b5cf6); }
.dialog-heading strong { display: block; color: #1e293b; font-size: 19px; line-height: 1.2; }
.dialog-heading span { display: block; margin-top: 4px; color: #94a3b8; font-size: 12px; }

.profile-hero {
  display: flex;
  gap: 16px;
  align-items: center;
  padding: 18px;
  border: 1px solid #dbeafe;
  border-radius: 18px;
  background: linear-gradient(135deg, #eff6ff, #f0fdfa);
}
.profile-avatar { flex: 0 0 auto; color: #fff; font-size: 24px; font-weight: 700; background: linear-gradient(135deg, #2563eb, #06b6d4); box-shadow: 0 10px 20px rgba(37, 99, 235, 0.25); }
.profile-main-copy { min-width: 0; flex: 1; }
.profile-name-row { gap: 8px; min-width: 0; }
.profile-name-row h2 { overflow: hidden; margin: 0; color: #1e3a8a; font-size: 20px; text-overflow: ellipsis; white-space: nowrap; }
.profile-main-copy p { overflow: hidden; margin: 5px 0 9px; color: #64748b; font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.connection-chip { display: inline-flex; gap: 6px; align-items: center; color: #64748b; font-size: 12px; }
.connection-chip .el-icon { color: #94a3b8; }
.connection-chip.online { color: #059669; }
.connection-chip.online .el-icon { color: #10b981; }

.profile-stat-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; margin: 16px 0; }
.profile-stat-item { min-width: 0; padding: 13px; border: 1px solid #edf2f7; border-radius: 14px; background: #fff; }
.profile-stat-item span, .settings-summary span, .profile-detail-list span { display: block; color: #94a3b8; font-size: 12px; }
.profile-stat-item strong { display: block; overflow: hidden; margin-top: 6px; color: #334155; font-size: 14px; text-overflow: ellipsis; white-space: nowrap; }
.profile-detail-card, .settings-section { border: 1px solid #e8eef7; border-radius: 16px; background: #fff; }
.profile-detail-card { padding: 16px; }
.section-title-row { justify-content: space-between; gap: 12px; }
.section-kicker { color: #60a5fa; font-size: 10px; font-weight: 700; letter-spacing: .12em; }
.section-title-row h3, .settings-section-title h3 { margin: 3px 0 0; color: #334155; font-size: 15px; }
.profile-detail-list { margin-top: 13px; border-top: 1px solid #f1f5f9; }
.profile-detail-list > div { display: flex; justify-content: space-between; gap: 20px; padding: 10px 0; border-bottom: 1px solid #f1f5f9; }
.profile-detail-list > div:last-child { border-bottom: 0; padding-bottom: 0; }
.profile-detail-list b { max-width: 68%; color: #475569; font-size: 13px; font-weight: 500; text-align: right; }
.profile-quick-actions { gap: 10px; margin-top: 16px; }
.profile-quick-actions .el-button { flex: 1; justify-content: space-between; }
.button-arrow { margin-left: 8px; }

.settings-summary { justify-content: space-between; gap: 14px; padding: 14px 16px; border-radius: 14px; background: #f8fafc; }
.settings-summary strong { display: block; margin-top: 4px; color: #334155; font-size: 14px; }
.settings-section { padding: 17px; margin-top: 14px; }
.settings-section-title { gap: 11px; }
.settings-section-icon { width: 34px; height: 34px; border-radius: 10px; font-size: 16px; }
.realtime-icon { background: linear-gradient(135deg, #0ea5e9, #14b8a6); }
.settings-section-title p { margin: 4px 0 0; color: #94a3b8; font-size: 12px; line-height: 1.45; }
.setting-row { justify-content: space-between; gap: 16px; padding: 14px 0; border-bottom: 1px solid #f1f5f9; }
.setting-row:last-child { padding-bottom: 0; border-bottom: 0; }
.setting-row strong, .realtime-status-row strong { display: block; color: #475569; font-size: 14px; }
.setting-row span, .realtime-status-row span { display: block; margin-top: 4px; color: #94a3b8; font-size: 12px; }
.realtime-status-row { gap: 10px; margin: 16px 0 14px; padding: 12px; border-radius: 12px; background: #f8fafc; }
.realtime-indicator { width: 9px; height: 9px; border-radius: 50%; background: #cbd5e1; box-shadow: 0 0 0 4px #e2e8f0; }
.realtime-indicator.online { background: #10b981; box-shadow: 0 0 0 4px #d1fae5; }
.settings-actions { gap: 8px; }
.settings-actions .el-icon { margin-right: 4px; }
.settings-footer-actions { justify-content: space-between; margin-top: 18px; }

.compact-layout .main-content { padding: 14px; }
.compact-layout .header { height: 62px; }
.reduce-motion *, .reduce-motion *::before, .reduce-motion *::after { transition-duration: .01ms !important; animation-duration: .01ms !important; }
@media (max-width: 1280px) {
  .header {
    padding: 0 16px;
  }

  .main-content {
    padding: 18px;
  }

  .sidebar-footer {
    display: none;
  }

  .user-info {
    padding: 8px 10px;
  }

  .username {
    display: none;
  }
}

@media (max-width: 1024px) {
  .header {
    padding: 0 14px;
    gap: 10px;
  }

  .header-left {
    display: block;
  }

  .header-right {
    gap: 10px;
  }
}


.mobile-page-title {
  font-size: 18px;
  font-weight: 700;
  color: #1d4ed8;
  letter-spacing: 0.01em;
}

.user-copy {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.user-role {
  font-size: 11px;
  color: #64748b;
}

.page-shell {
  width: min(100%, 1680px);
  margin: 0 auto;
}

.settings-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}

@media (max-width: 1280px) {
  .user-copy {
    display: none;
  }

  .page-shell {
    width: 100%;
  }
}

@media (max-width: 768px) {  :deep(.profile-dialog),
  :deep(.settings-dialog) {
    margin-top: 7vh !important;
    max-height: 84dvh;
  }

  :deep(.profile-dialog .el-dialog__header),
  :deep(.settings-dialog .el-dialog__header) {
    padding: 18px 18px 13px;
  }

  :deep(.profile-dialog .el-dialog__body),
  :deep(.settings-dialog .el-dialog__body) {
    max-height: calc(84dvh - 75px);
    padding: 16px 18px 20px;
    overflow-y: auto;
  }

  .dialog-heading strong { font-size: 17px; }
  .profile-hero { padding: 14px; }
  .profile-stat-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .profile-stat-item:last-child { grid-column: span 2; }
  .profile-quick-actions { flex-direction: column; align-items: stretch; }
  .profile-quick-actions .el-button { flex: initial; }
  .setting-row { align-items: flex-start; }
  .setting-row > div { padding-right: 4px; }
  .settings-footer-actions { margin-top: 14px; }
  .layout-container {
    height: 100dvh;
    flex-direction: column;
  }

  .sidebar {
    width: 100% !important;
    height: auto;
    order: 2;
    border-right: none;
    border-top: 1px solid rgba(255, 255, 255, 0.6);
    box-shadow: 0 -10px 24px rgba(37, 99, 235, 0.12);
  }

  .logo-container,
  .sidebar-footer,
  .footer,
  .hidden-mobile-control,
  .user-arrow {
    display: none;
  }

  .sidebar-menu {
    display: flex;
    align-items: stretch;
    justify-content: space-between;
    gap: 8px;
    padding: 8px 10px calc(8px + env(safe-area-inset-bottom));
    overflow-x: auto;
    border-top: none;
  }

  .sidebar-menu :deep(.el-menu-item) {
    flex: 1 0 68px;
    min-width: 68px;
    min-height: 56px;
    height: auto;
    line-height: 1.15;
    margin: 0;
    padding: 10px 8px !important;
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 6px;
  }

  .sidebar-menu :deep(.el-menu-item:hover) {
    transform: none;
  }

  .sidebar-menu :deep(.el-menu-item.is-active::before) {
    left: 50%;
    top: auto;
    bottom: 4px;
    width: 24px;
    height: 3px;
    transform: translateX(-50%);
    border-radius: 999px;
  }

  .sidebar-menu :deep(.el-menu-item span) {
    margin: 0;
    font-size: 12px;
    text-align: center;
    white-space: normal;
  }

  .sidebar-menu :deep(.el-icon) {
    margin: 0;
    font-size: 18px;
  }

  .main-container {
    order: 1;
    min-height: 0;
  }

  .main-content {
    padding: 12px 12px calc(94px + env(safe-area-inset-bottom));
  }

  .header {
    height: 64px;
    padding: 0 12px;
  }

  .header-right {
    gap: 8px;
  }

  .user-info {
    padding: 6px 10px;
  }

  .mobile-page-title {
    font-size: 17px;
  }
}

</style>
