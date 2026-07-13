<template>
  <div class="reset-container">
    <div class="reset-card glass-effect">
      <div class="card-header">
        <div class="logo">
          <el-icon
            :size="48"
            color="#fff"
          >
            <Lock />
          </el-icon>
        </div>
        <h2>找回密码</h2>
        <p class="subtitle">
          使用用户名和注册邮箱重置登录密码
        </p>
      </div>

      <el-alert
        title="验证码会发送到注册邮箱；如果账号未绑定邮箱，请联系管理员在后台重置密码。"
        type="info"
        show-icon
        :closable="false"
        class="reset-tip"
      />

      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        class="reset-form"
        size="large"
        @keyup.enter="handleReset"
      >
        <el-form-item prop="username">
          <el-input
            v-model="form.username"
            placeholder="请输入用户名"
            :prefix-icon="User"
            clearable
          />
        </el-form-item>

        <el-form-item prop="email">
          <el-input
            v-model="form.email"
            placeholder="请输入注册邮箱"
            :prefix-icon="Message"
            clearable
          />
        </el-form-item>

        <el-form-item prop="code">
          <el-input
            v-model="form.code"
            placeholder="请输入邮箱验证码"
            :prefix-icon="Message"
            maxlength="6"
            clearable
          >
            <template #append>
              <el-button
                :loading="sendingCode"
                :disabled="codeCountdown > 0"
                @click="handleSendCode"
              >
                {{ codeCountdown > 0 ? `${codeCountdown}s后重发` : '发送验证码' }}
              </el-button>
            </template>
          </el-input>
        </el-form-item>

        <el-form-item prop="newPassword">
          <el-input
            v-model="form.newPassword"
            type="password"
            placeholder="请输入新密码"
            :prefix-icon="Lock"
            show-password
            clearable
          />
        </el-form-item>

        <el-form-item prop="confirmPassword">
          <el-input
            v-model="form.confirmPassword"
            type="password"
            placeholder="请再次输入新密码"
            :prefix-icon="Lock"
            show-password
            clearable
          />
        </el-form-item>

        <el-form-item>
          <el-button
            type="primary"
            :loading="loading"
            class="submit-btn"
            size="large"
            @click="handleReset"
          >
            重置密码
          </el-button>
        </el-form-item>

        <div class="login-link">
          <span>想起密码了？</span>
          <router-link
            to="/login"
            class="gradient-text"
          >
            返回登录
          </router-link>
        </div>
      </el-form>
    </div>

    <div class="bg-decoration">
      <div class="circle circle-1" />
      <div class="circle circle-2" />
      <div class="circle circle-3" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { onUnmounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Lock, Message, User } from '@element-plus/icons-vue'
import { resetPassword, sendPasswordResetCode } from '@/api/auth'

const router = useRouter()
const formRef = ref<FormInstance>()
const loading = ref(false)
const sendingCode = ref(false)
const codeCountdown = ref(0)
let countdownTimer: number | undefined

const form = reactive({
  username: '',
  email: '',
  code: '',
  newPassword: '',
  confirmPassword: ''
})

const validateConfirmPassword = (_rule: unknown, value: string, callback: (error?: Error) => void) => {
  if (value !== form.newPassword) {
    callback(new Error('两次输入的新密码不一致'))
    return
  }
  callback()
}

const rules = reactive<FormRules>({
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 3, max: 50, message: '用户名长度应为3-50个字符', trigger: 'blur' }
  ],
  email: [
    { required: true, message: '请输入注册邮箱', trigger: 'blur' },
    { type: 'email', message: '请输入有效邮箱地址', trigger: 'blur' }
  ],
  code: [
    { required: true, message: '请输入邮箱验证码', trigger: 'blur' },
    { pattern: /^\d{6}$/, message: '验证码应为6位数字', trigger: 'blur' }
  ],
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 12, message: '密码长度不能少于12个字符', trigger: 'blur' }
  ],
  confirmPassword: [
    { required: true, message: '请再次输入新密码', trigger: 'blur' },
    { validator: validateConfirmPassword, trigger: 'blur' }
  ]
})

function startCountdown() {
  codeCountdown.value = 60
  if (countdownTimer) {
    window.clearInterval(countdownTimer)
  }
  countdownTimer = window.setInterval(() => {
    codeCountdown.value -= 1
    if (codeCountdown.value <= 0 && countdownTimer) {
      window.clearInterval(countdownTimer)
      countdownTimer = undefined
    }
  }, 1000)
}

async function validateResetIdentity() {
  if (!formRef.value) return false
  try {
    await formRef.value.validateField(['username', 'email'])
    return true
  } catch {
    return false
  }
}

async function handleSendCode() {
  if (sendingCode.value || codeCountdown.value > 0) return
  if (!(await validateResetIdentity())) return

  sendingCode.value = true
  try {
    await sendPasswordResetCode({
      username: form.username,
      email: form.email
    })
    ElMessage.success('如果用户名和邮箱匹配，验证码将发送到该邮箱')
    startCountdown()
  } catch (error: any) {
    ElMessage.error(error.response?.data?.message || '验证码发送失败')
  } finally {
    sendingCode.value = false
  }
}

async function handleReset() {
  if (!formRef.value) return

  await formRef.value.validate(async (valid) => {
    if (!valid) return
    loading.value = true
    try {
      await resetPassword({
        username: form.username,
        email: form.email,
        code: form.code,
        new_password: form.newPassword
      })
      ElMessage.success('密码已重置，请使用新密码登录')
      router.push('/login')
    } catch (error: any) {
      ElMessage.error(error.response?.data?.message || '密码重置失败')
    } finally {
      loading.value = false
    }
  })
}

onUnmounted(() => {
  if (countdownTimer) {
    window.clearInterval(countdownTimer)
  }
})
</script>

<style scoped>
.reset-container {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #e0f2fe 0%, #bae6fd 50%, #7dd3fc 100%);
  position: relative;
  overflow: hidden;
}

.glass-effect {
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.5);
  box-shadow: 0 8px 32px 0 rgba(59, 130, 246, 0.15);
}

.reset-card {
  width: 440px;
  padding: 40px;
  border-radius: 20px;
  position: relative;
  z-index: 10;
}

.card-header {
  text-align: center;
  margin-bottom: 24px;
}

.logo {
  width: 80px;
  height: 80px;
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 50%, #06b6d4 100%);
  border-radius: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 20px;
  box-shadow: 0 8px 20px rgba(59, 130, 246, 0.3);
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

.reset-tip {
  margin-bottom: 20px;
}

.reset-form :deep(.el-input__wrapper) {
  border-radius: 10px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.reset-form :deep(.el-input__inner) {
  height: 44px;
}

.submit-btn {
  width: 100%;
  height: 48px;
  border-radius: 10px;
  font-size: 16px;
  font-weight: 500;
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 50%, #06b6d4 100%);
  border: none;
  box-shadow: 0 4px 15px rgba(59, 130, 246, 0.3);
}

.login-link {
  text-align: center;
  margin-top: 20px;
  color: #666;
  font-size: 14px;
}

.gradient-text {
  margin-left: 6px;
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 50%, #06b6d4 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  text-decoration: none;
  font-weight: 500;
}

.bg-decoration {
  position: absolute;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
}

.circle {
  position: absolute;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.3);
  backdrop-filter: blur(10px);
}

.circle-1 {
  width: 300px;
  height: 300px;
  top: -100px;
  right: -100px;
}

.circle-2 {
  width: 200px;
  height: 200px;
  bottom: -50px;
  left: -50px;
}

.circle-3 {
  width: 150px;
  height: 150px;
  top: 50%;
  left: 10%;
}

@media (max-width: 480px) {
  .reset-card {
    width: calc(100% - 32px);
    padding: 28px 20px;
  }
}
</style>
