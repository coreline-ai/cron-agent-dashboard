import { defineConfig, devices } from '@playwright/test';

const port = Number(process.env.E2E_PORT ?? 18083);
const baseURL = process.env.E2E_BASE_URL ?? `http://127.0.0.1:${port}`;

export default defineConfig({
  testDir: './tests/e2e',
  timeout: 60_000,
  expect: {
    timeout: 10_000
  },
  use: {
    baseURL,
    trace: 'retain-on-failure'
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] }
    }
  ],
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : {
        command: `rm -rf .tmp/e2e-data && mkdir -p .tmp && ./corn-agent-dashboard serve --data-dir .tmp/e2e-data --bind 127.0.0.1:${port}`,
        url: `${baseURL}/healthz`,
        timeout: 120_000,
        reuseExistingServer: false
      },
  reporter: process.env.CI ? [['github'], ['list']] : 'list'
});
