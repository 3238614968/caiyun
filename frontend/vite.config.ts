import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'
import * as ElementPlusIcons from '@element-plus/icons-vue'
import { resolve } from 'path'
import { uiPreviewPlugin } from './scripts/ui-preview'

const elementPlusIconResolver = (name: string) => {
  // Resolve only exports actually provided by Element Plus. This removes the
  // maintenance-prone handwritten icon allowlist while avoiding imports for
  // arbitrary component names.
  if (!(name in ElementPlusIcons)) return undefined
  return {
    name,
    from: '@element-plus/icons-vue'
  }
}

export default defineConfig(({ command, mode }) => ({
  define: {
    'import.meta.env.VITE_UI_PREVIEW': JSON.stringify(String(
      process.env.CAIYUN_UI_PREVIEW === '1' && (command === 'serve' || mode === 'ui-preview')
    ))
  },
  plugins: [
    ...(command === 'serve' && process.env.CAIYUN_UI_PREVIEW === '1' ? [uiPreviewPlugin()] : []),
    vue(),
    Components({
      dts: 'src/components.d.ts',
      resolvers: [
        ElementPlusResolver({ importStyle: 'css' }),
        elementPlusIconResolver
      ]
    })
  ],
  optimizeDeps: { include: ['element-plus/es', '@element-plus/icons-vue', 'vue-router', 'pinia', 'axios'] },
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.test.ts'],
    server: { deps: { inline: ['element-plus'] } }
  },
  build: {
    chunkSizeWarningLimit: 1200,
    rollupOptions: {
      output: {
        chunkFileNames: chunk => chunk.isDynamicEntry ? 'assets/[name]-[hash].js' : 'assets/[hash].js',
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
      '/events': {
        // EventSource uses a regular long-lived HTTP request, not WebSocket.
        target: 'http://localhost:8080',
        changeOrigin: true
      },
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true
      }
    }
  }
}))
