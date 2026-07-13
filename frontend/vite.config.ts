import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'
import { resolve } from 'path'

const elementPlusIconNames = new Set([
  'ArrowDown',
  'Bell',
  'Box',
  'CircleCheck',
  'CircleClose',
  'Cloudy',
  'Close',
  'DataLine',
  'Document',
  'Download',
  'Expand',
  'Fold',
  'FullScreen',
  'HomeFilled',
  'InfoFilled',
  'List',
  'Lock',
  'Message',
  'PieChart',
  'Plus',
  'Present',
  'Refresh',
  'Search',
  'Setting',
  'Shop',
  'SwitchButton',
  'Timer',
  'User',
  'Warning'
])

const elementPlusIconResolver = (name: string) => {
  if (!elementPlusIconNames.has(name)) return undefined
  return {
    name,
    from: '@element-plus/icons-vue'
  }
}

export default defineConfig({
  plugins: [
    vue(),
    Components({
      dts: 'src/components.d.ts',
      resolvers: [
        ElementPlusResolver({ importStyle: 'css' }),
        elementPlusIconResolver
      ]
    })
  ],
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.test.ts']
  },
  build: {
    chunkSizeWarningLimit: 1200,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined
          if (id.includes('/vue') || id.includes('\\vue') || id.includes('vue-router') || id.includes('pinia')) {
            return 'vue'
          }
          return undefined
        }
      }
    }
  },
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src')
    }
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      },
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true
      }
    }
  }
})
