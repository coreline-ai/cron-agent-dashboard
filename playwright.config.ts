import { defineConfig, devices } from '@playwright/test';

const port = Number(process.env.E2E_PORT ?? 18083);
const baseURL = process.env.E2E_BASE_URL ?? `http://127.0.0.1:${port}`;

export default defineConfig({
  testDir: './tests/e2e',
  timeout: 60_000,
  retries: process.env.CI ? 2 : 1,
  // E2E tests share one local server and SQLite data-dir. Keep the suite
  // serial by default so global setup/cleanup scenarios do not race each other.
  workers: Number(process.env.E2E_WORKERS ?? 1),
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
