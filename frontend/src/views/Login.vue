<template>
  <div class="login-container">
    <span
      v-if="uiPreview"
      class="preview-label auth-preview-label"
    >交互预览 · 示例数据</span>
    <div class="login-card glass-effect">
      <div class="card-header">
        <div class="logo">
          <el-icon
            :size="48"
            color="#409eff"
          >
            <Cloudy />
          </el-icon>
        </div>
        <h2>欢迎回来</h2>
        <p class="subtitle">
          登录移动云盘管理系统
        </p>
      </div>

      <p
        v-if="authStore.logoutPending"
        role="status"
      >
        服务端尚未确认退出。
        <el-button
          type="primary"
          link
          :loading="authStore.loggingOut"
          @click="retryLogout"
        >
          重试退出
        </el-button>
      </p>

      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        class="login-form"
        size="large"
        @keyup.enter="handleLogin"
      >
        <el-form-item prop="username">
          <el-input
            v-model="form.username"
            aria-label="用户名"
            autocomplete="username"
            placeholder="请输入用户名"
            :prefix-icon="User"
            clearable
          />
        </el-form-item>

        <el-form-item prop="password">
          <el-input
            v-model="form.password"
            aria-label="密码"
            autocomplete="current-password"
            type="password"
            placeholder="请输入密码"
            :prefix-icon="Lock"
            show-password
            clearable
          />
        </el-form-item>

        <div class="forgot-link">
          <router-link
            to="/forgot-password"
            class="gradient-text"
          >
            忘记密码？
          </router-link>
        </div>

        <el-form-item>
          <el-button
            type="primary"
            :loading="loading"
            :disabled="authStore.loggingOut"
            class="submit-btn"
            size="large"
            @click="handleLogin"
          >
            登录
          </el-button>
        </el-form-item>

        <div class="register-link">
          <span>还没有账号？</span>
          <router-link
            to="/register"
            class="gradient-text"
          >
            立即注册
          </router-link>
        </div>
      </el-form>
    </div>

    <!-- 背景装饰 -->
    <div class="bg-decoration">
      <div class="circle circle-1" />
      <div class="circle circle-2" />
      <div class="circle circle-3" />
    </div>
  </div>
</template>

<script setup lang="ts">
const uiPreview = import.meta.env.VITE_UI_PREVIEW === 'true'
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { User, Lock, Cloudy } from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'

const router = useRouter()
const authStore = useAuthStore()
const formRef = ref<FormInstance>()
const loading = ref(false)

const form = reactive({
  username: '',
  password: ''
})

const rules = reactive<FormRules>({
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 3, max: 50, message: '用户名长度应为3-50个字符', trigger: 'blur' }
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 1, message: '请输入密码', trigger: 'blur' }
  ]
})

async function retryLogout() {
  try {
    await authStore.logout()
    ElMessage.success('已退出登录')
  } catch {
    ElMessage.warning('退出尚未确认，请恢复连接后重试')
  }
}

async function handleLogin() {
  if (!formRef.value) return

  await formRef.value.validate(async (valid) => {
    if (valid) {
      loading.value = true
      try {
        await authStore.login(form.username, form.password)
        ElMessage.success('登录成功')
        router.push('/')
      } catch (error: any) {
        ElMessage.error(error.response?.data?.message || '登录失败')
      } finally {
        loading.value = false
      }
    }
  })
}
</script>

<style scoped>
.login-container {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #f0f9ff 0%, #e0f2fe 55%, #d9ecff 100%);
  position: relative;
  overflow: hidden;
}

/* 毛玻璃效果卡片 */
.glass-effect {
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.5);
  box-shadow: 0 8px 32px 0 rgba(15, 23, 42, 0.15);
}

.login-card {
  width: 420px;
  padding: 40px;
  border-radius: 20px;
  position: relative;
  z-index: 10;
}

.card-header {
  text-align: center;
  margin-bottom: 30px;
}

.logo {
  width: 80px;
  height: 80px;
  background: #ecf5ff;
  border-radius: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 20px;
  box-shadow: 0 8px 20px rgba(15, 23, 42, 0.3);
}

.card-header h2 {
  margin: 0 0 8px;
  font-size: 28px;
  color: #333;
  font-weight: 600;
}

.subtitle {
  color: #666;
  font-size: 14px;
  margin: 0;
}

.login-form :deep(.el-input__wrapper) {
  border-radius: 10px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.login-form :deep(.el-input__inner) {
  height: 44px;
}

.submit-btn {
  width: 100%;
  height: 48px;
  border-radius: 10px;
  font-size: 16px;
  font-weight: 500;
  background: #409eff;
  border: none;
  transition: all 0.3s ease;
}

.submit-btn:hover { background: #66b1ff; }

.register-link {
  text-align: center;
  margin-top: 20px;
  color: #666;
  font-size: 14px;
}

.forgot-link {
  text-align: right;
  margin: -8px 0 18px;
  font-size: 14px;
}

.gradient-text { margin-left: 6px; color: #409eff; text-decoration: none; font-weight: 500;}

.gradient-text:hover {
  text-decoration: underline;
}

/* 背景装饰 */
.bg-decoration {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  overflow: hidden;
}

.circle {
  position: absolute;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.4);
  animation: float 20s infinite ease-in-out;
}

.circle-1 {
  width: 300px;
  height: 300px;
  top: -100px;
  right: -100px;
  animation-delay: 0s;
}

.circle-2 {
  width: 200px;
  height: 200px;
  bottom: -50px;
  left: -50px;
  animation-delay: -5s;
}

.circle-3 {
  width: 150px;
  height: 150px;
  bottom: 20%;
  right: 10%;
  animation-delay: -10s;
}

@keyframes float {
  0%, 100% {
    transform: translate(0, 0) scale(1);
  }
  33% {
    transform: translate(30px, -30px) scale(1.1);
  }
  66% {
    transform: translate(-20px, 20px) scale(0.9);
  }
}

/* 响应式 */
@media (max-width: 480px) {
  .login-card {
    width: 90%;
    padding: 30px 20px;
  }
}
</style>
