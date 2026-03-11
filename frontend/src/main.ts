import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'

// 引入移动端适配样式
import './styles/mobile.css'

import App from './App.vue'
import router from './router'

const app = createApp(App)

// 注册Element Plus图标
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

const pinia = createPinia()
app.use(pinia)

// Initialize auth store from localStorage (must be after pinia is installed)
import { useAuthStore } from './store/auth'
const authStore = useAuthStore()
authStore.initialize()

app.use(router)
app.use(ElementPlus, { locale: zhCn })

app.mount('#app')
