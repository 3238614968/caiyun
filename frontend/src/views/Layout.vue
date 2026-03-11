<template>
  <el-container class="layout-container">
    <!-- 侧边栏 -->
    <el-aside :width="isCollapse ? '64px' : '220px'" class="sidebar">
      <div class="logo-container">
        <div class="logo">
          <el-icon :size="32" color="#fff"><Cloudy /></el-icon>
        </div>
        <span v-show="!isCollapse" class="logo-text">移动云盘</span>
      </div>

      <el-menu
        :default-active="activeMenu"
        :collapse="isCollapse"
        :collapse-transition="false"
        router
        class="sidebar-menu"
        background-color="transparent"
        text-color="#fff"
        active-text-color="#fff"
      >
        <el-menu-item index="/dashboard">
          <el-icon><DataLine /></el-icon>
          <template #title>仪表盘</template>
        </el-menu-item>

        <el-menu-item index="/accounts">
          <el-icon><User /></el-icon>
          <template #title>账号管理</template>
        </el-menu-item>

        <el-menu-item index="/logs">
          <el-icon><List /></el-icon>
          <template #title>运行日志</template>
        </el-menu-item>

        <el-menu-item index="/exchange">
          <el-icon><Shop /></el-icon>
          <template #title>兑换中心</template>
        </el-menu-item>

        <el-menu-item v-if="isAdmin" index="/admin">
          <el-icon><Setting /></el-icon>
          <template #title>管理员面板</template>
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
          <breadcrumb />
        </div>

        <div class="header-right">
          <!-- 通知中心 -->
          <NotificationCenter />

          <!-- 全屏 -->
          <el-button type="text" class="header-btn" @click="toggleFullscreen">
            <el-icon :size="20"><FullScreen /></el-icon>
          </el-button>

          <!-- 用户菜单 -->
          <el-dropdown @command="handleCommand" class="user-dropdown">
            <div class="user-info">
              <el-avatar :size="36" class="user-avatar">
                {{ userInitials }}
              </el-avatar>
              <span class="username">{{ authStore.user?.username }}</span>
              <el-icon><ArrowDown /></el-icon>
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
        <router-view v-slot="{ Component }">
          <transition name="fade-transform" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
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
import { wsClient } from '@/api/websocket'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const profileVisible = ref(false)
const settingsVisible = ref(false)

// 侧边栏折叠状态
const isCollapse = ref(false)

// 当前激活的菜单
const activeMenu = computed(() => route.path)

// 是否为管理员
const isAdmin = computed(() => authStore.user?.role === 'admin')

// 用户头像文字
const userInitials = computed(() => {
  const username = authStore.user?.username || ''
  return username.charAt(0).toUpperCase()
})

// 切换侧边栏折叠
const toggleCollapse = () => {
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

onMounted(() => {
  // 用户已登录时建立WebSocket连接
  if (authStore.token) {
    wsClient.connect()
  }
})

onUnmounted(() => {
  wsClient.disconnect()
})
</script>

<style scoped>
.layout-container {
  height: 100vh;
  background: #f5f7fa;
}

/* 侧边栏 */
.sidebar {
  background: linear-gradient(180deg, #e0f2fe 0%, #bae6fd 50%, #7dd3fc 100%);
  transition: width 0.3s;
  display: flex;
  flex-direction: column;
  position: relative;
}

.logo-container {
  height: 70px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 20px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.logo {
  width: 40px;
  height: 40px;
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 50%, #06b6d4 100%);
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  box-shadow: 0 4px 10px rgba(59, 130, 246, 0.3);
}

.logo-text {
  margin-left: 12px;
  font-size: 20px;
  font-weight: 600;
  color: #1e40af;
  white-space: nowrap;
}

.sidebar-menu {
  flex: 1;
  border-right: none;
  padding: 20px 0;
}

.sidebar-menu :deep(.el-menu-item) {
  margin: 8px 12px;
  border-radius: 10px;
  height: 50px;
  line-height: 50px;
  transition: all 0.3s;
  color: #1e40af !important;
}

.sidebar-menu :deep(.el-menu-item:hover) {
  background: rgba(59, 130, 246, 0.1) !important;
}

.sidebar-menu :deep(.el-menu-item.is-active) {
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 100%) !important;
  color: #fff !important;
  box-shadow: 0 4px 15px rgba(59, 130, 246, 0.3);
}

.sidebar-menu :deep(.el-icon) {
  font-size: 20px;
  margin-right: 12px;
}

.sidebar-footer {
  padding: 15px;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  display: flex;
  justify-content: center;
}

.collapse-btn {
  color: #1e40af;
  padding: 8px;
  border-radius: 8px;
  transition: all 0.3s;
}

.collapse-btn:hover {
  background: rgba(59, 130, 246, 0.1);
}

/* 主容器 */
.main-container {
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* 顶部导航 */
.header {
  height: 70px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  background: rgba(255, 255, 255, 0.95);
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

.glass-effect-light {
  background: rgba(255, 255, 255, 0.95);
}

.header-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.header-btn {
  color: #666;
  padding: 8px;
  border-radius: 8px;
  transition: all 0.3s;
}

.header-btn:hover {
  background: rgba(59, 130, 246, 0.1);
  color: #3b82f6;
}


.user-dropdown {
  cursor: pointer;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 12px;
  border-radius: 10px;
  transition: all 0.3s;
}

.user-info:hover {
  background: rgba(59, 130, 246, 0.1);
}

.user-avatar {
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 50%, #06b6d4 100%);
  color: #fff;
  font-weight: 600;
}

.username {
  font-size: 14px;
  color: #333;
  font-weight: 500;
}

/* 内容区 */
.main-content {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
  background: #f5f7fa;
}

/* 页脚 */
.footer {
  height: 50px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #fff;
  border-top: 1px solid #e4e7ed;
  color: #999;
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
</style>
