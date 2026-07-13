import { createApp } from 'vue'
import { ElLoading } from 'element-plus'
import { createPinia } from 'pinia'
import './plugins/element-styles'

// 引入移动端适配样式
import './styles/mobile.css'
import App from './App.vue'
import router from './router'

const app = createApp(App)

const pinia = createPinia()
app.use(pinia)

// Initialize auth store from localStorage (must be after pinia is installed)
import { useAuthStore } from './store/auth'
const authStore = useAuthStore()
authStore.initialize()

app.use(router)
app.use(ElLoading)

app.mount('#app')
