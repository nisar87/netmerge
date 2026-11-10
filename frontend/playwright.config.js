import { defineConfig } from '@playwright/test';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const frontend = dirname(fileURLToPath(import.meta.url));
const port = Number(process.env.NETMERGE_FIXTURE_PORT || process.env.PORT || 4173);
const baseURL = `http://127.0.0.1:${port}`;

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  // The fixture API is one mutable local process; reset and writes must be serialized.
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  timeout: 30_000,
  expect: { timeout: 7_000 },
  outputDir: process.env.NETMERGE_TEST_OUTPUT || resolve('/tmp', `netmerge-playwright-results-${port}`),
  reporter: [['list']],
  use: {
    baseURL,
    browserName: 'chromium',
    viewport: { width: 1440, height: 900 },
    locale: 'en-US', timezoneId: 'UTC', colorScheme: 'light', reducedMotion: 'reduce',
    serviceWorkers: 'block',
    actionTimeout: 7_000,
    navigationTimeout: 15_000,
    trace: 'retain-on-failure', screenshot: 'only-on-failure',
    ...(process.env.NETMERGE_BROWSER_EXECUTABLE ? { launchOptions: { executablePath: process.env.NETMERGE_BROWSER_EXECUTABLE } } : {}),
  },
  webServer: {
    command: 'node scripts/serve.mjs',
    cwd: frontend,
    env: { NETMERGE_FIXTURE_PORT: String(port), NETMERGE_FIXTURE_HOST: '127.0.0.1' },
    url: `${baseURL}/__test/health`,
    reuseExistingServer: false,
    timeout: 15_000,
    gracefulShutdown: { signal: 'SIGTERM', timeout: 3_000 },
  },
});
