import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/store/auth'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/views/Login.vue'),
      meta: { requiresAuth: false }
    },
    {
      path: '/register',
      name: 'Register',
      component: () => import('@/views/Register.vue'),
      meta: { requiresAuth: false }
    },
    {
      path: '/forgot-password',
      name: 'ForgotPassword',
      component: () => import('@/views/ForgotPassword.vue'),
      meta: { requiresAuth: false }
    },
    {
      path: '/',
      name: 'Layout',
      component: () => import('@/views/Layout.vue'),
      meta: { requiresAuth: true },
      redirect: '/dashboard',
      children: [
        {
          path: 'dashboard',
          name: 'Dashboard',
          component: () => import('@/views/Dashboard.vue'),
          meta: { title: '首页' }
        },
        {
          path: 'accounts',
          name: 'Accounts',
          component: () => import('@/views/AccountManage.vue'),
          meta: { title: '账号' }
        },
        {
          path: 'logs',
          name: 'Logs',
          component: () => import('@/views/TaskLogs.vue'),
          meta: { title: '日志' }
        },
        {
          path: 'admin',
          name: 'Admin',
          component: () => import('@/views/AdminPanel.vue'),
          meta: { title: '管理', requiresAdmin: true }
        },
        {
          path: 'exchange',
          name: 'Exchange',
          component: () => import('@/views/ExchangeCenter.vue'),
          meta: { title: '兑换' }
        },
        {
          path: 'exchange/records',
          name: 'ExchangeRecords',
          component: () => import('@/views/ExchangeRecords.vue'),
          meta: { title: '抢兑记录' }
        }
      ]
    }
  ]
})

router.beforeEach((to, _from, next) => {
  const authStore = useAuthStore()
  const isAuthenticated = authStore.isAuthenticated
  const userRole = authStore.user?.role

  if (to.meta.requiresAuth && !isAuthenticated) {
    next('/login')
  } else if (to.meta.requiresAdmin && userRole !== 'admin') {
    next('/')
  } else if ((to.name === 'Login' || to.name === 'Register') && isAuthenticated) {
    next('/')
  } else {
    next()
  }
})

export default router
