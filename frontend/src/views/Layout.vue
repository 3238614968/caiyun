<template>
  <el-container class="layout-container">
    <!-- 侧边栏 -->
    <el-aside :width="asideWidth" class="sidebar">
      <div class="logo-container">
        <div class="logo">
          <el-icon :size="28" color="#fff"><Cloudy /></el-icon>
        </div>
        <span v-show="!menuCollapsed" class="logo-text">移动云盘</span>
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
          <template #title>首页</template>
        </el-menu-item>

        <el-menu-item index="/accounts">
          <el-icon><User /></el-icon>
          <template #title>账号</template>
        </el-menu-item>

        <el-menu-item index="/logs">
          <el-icon><List /></el-icon>
          <template #title>日志</template>
        </el-menu-item>

        <el-menu-item index="/exchange">
          <el-icon><Shop /></el-icon>
          <template #title>兑换</template>
        </el-menu-item>

        <el-menu-item v-if="isAdmin" index="/admin">
          <el-icon><Setting /></el-icon>
          <template #title>管理</template>
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
          <div v-else class="mobile-page-title">{{ currentTitle }}</div>
        </div>

        <div class="header-right">
          <!-- 通知中心 -->
          <NotificationCenter />

          <!-- 全屏 -->
          <el-button type="text" class="header-btn hidden-mobile-control" @click="toggleFullscreen">
            <el-icon :size="20"><FullScreen /></el-icon>
          </el-button>

          <!-- 用户菜单 -->
          <el-dropdown @command="handleCommand" class="user-dropdown">
            <div class="user-info">
              <el-avatar :size="isMobileViewport ? 34 : 36" class="user-avatar">
                {{ userInitials }}
              </el-avatar>
              <div class="user-copy">
                <span class="username">{{ authStore.user?.username }}</span>
                <span class="user-role">{{ isAdmin ? '管理员' : '普通用户' }}</span>
              </div>
              <el-icon class="user-arrow"><ArrowDown /></el-icon>
            </div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="profile">
                  <el-icon><User /></el-icon>个人中心
                </el-dropdown-item>
                <el-dropdown-item command="settings">
                  <el-icon><Setting /></el-icon>系统设置
                </el-dropdown-item>
                <el-dropdown-item divided command="logout">
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
            <transition name="fade-transform" mode="out-in">
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

  <el-dialog v-model="profileVisible" title="个人中心" width="520px">
    <el-descriptions :column="1" border>
      <el-descriptions-item label="用户名">{{ authStore.user?.username || '-' }}</el-descriptions-item>
      <el-descriptions-item label="角色">{{ authStore.user?.role || '-' }}</el-descriptions-item>
      <el-descriptions-item label="用户 ID">{{ (authStore.user as any)?.id ?? '-' }}</el-descriptions-item>
      <el-descriptions-item label="WebSocket 状态">
        <el-tag :type="wsClient.connected.value ? 'success' : 'danger'">
          {{ wsClient.connected.value ? '已连接' : '未连接' }}
        </el-tag>
      </el-descriptions-item>
    </el-descriptions>
    <template #footer>
      <el-button @click="profileVisible = false">关闭</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="settingsVisible" title="系统设置" width="560px">
    <el-form label-width="120px">
      <el-form-item label="侧边栏折叠">
        <el-switch v-model="isCollapse" active-text="折叠" inactive-text="展开" />
      </el-form-item>
      <el-form-item label="WebSocket">
        <div style="display: flex; gap: 8px; align-items: center;">
          <el-tag :type="wsClient.connected.value ? 'success' : 'info'">
            {{ wsClient.connected.value ? '已连接' : '未连接' }}
          </el-tag>
          <el-button size="small" @click="wsClient.connect">重连</el-button>
          <el-button size="small" @click="wsClient.disconnect">断开</el-button>
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="settingsVisible = false">关闭</el-button>
    </template>
  </el-dialog>

  <!-- 公告弹窗 -->
  <AnnouncementPopup
    v-model="popupVisible"
    :announcement="popupAnnouncement"
    :can-dismiss="true"
    @dismiss="handlePopupDismiss"
  />
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
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
  Bell,
  FullScreen,
  ArrowDown,
  SwitchButton
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

// 侧边栏折叠状态
const isCollapse = ref(false)
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

// 用户头像文字
const userInitials = computed(() => {
  const username = authStore.user?.username || ''
  return username.charAt(0).toUpperCase()
})

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
        authStore.logout()
        router.push('/login')
        ElMessage.success('已退出登录')
      } catch {
        // 用户取消
      }
      break
  }
}

// 弹窗公告队列
const popupQueue = ref<Announcement[]>([])
const currentPopupIndex = ref(0)

// 检查并显示弹窗公告
const checkPopupAnnouncement = async () => {
  try {
    const res: any = await getPopupAnnouncement()
    if (res.has_popup && res.announcements && res.announcements.length > 0) {
      // 检查用户已经关闭过的公告
      const dismissedAnnouncements = JSON.parse(localStorage.getItem('dismissedAnnouncements') || '[]')
      // 过滤出未关闭的公告
      const unreadAnnouncements = res.announcements.filter(
        (a: Announcement) => !dismissedAnnouncements.includes(a.id)
      )
      
      if (unreadAnnouncements.length > 0) {
        popupQueue.value = unreadAnnouncements
        currentPopupIndex.value = 0
        showNextPopup()
      }
    }
  } catch (error) {
    // 忽略错误
  }
}

// 显示下一个弹窗公告
const showNextPopup = () => {
  if (currentPopupIndex.value < popupQueue.value.length) {
    popupAnnouncement.value = popupQueue.value[currentPopupIndex.value]
    popupVisible.value = true
  }
}

const handlePopupDismiss = () => {
  popupVisible.value = false
  // 显示下一个公告
  currentPopupIndex.value++
  showNextPopup()
}

onMounted(() => {
  syncViewport()
  window.addEventListener('resize', syncViewport)
  // 用户已登录时建立WebSocket连接
  if (authStore.token) {
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

@media (max-width: 768px) {
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


