import { defineConfig, devices } from '@playwright/test'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://frontend:8080'

export default defineConfig({
  testDir: './e2e-real',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  timeout: 45_000,
  expect: {
    timeout: 10_000
  },
  reporter: process.env.CI
    ? [
        ['list'],
        ['html', { open: 'never' }]
      ]
    : 'list',
  use: {
    baseURL,
    locale: 'zh-CN',
    timezoneId: 'Asia/Shanghai',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure'
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1440, height: 900 }
      }
    }
  ]
})
