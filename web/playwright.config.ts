import { defineConfig } from '@playwright/test';
import { e2eDataDir } from './tests/e2e/data-dir.js';

const port = 8486;
const dataDir = e2eDataDir();

// End-to-end tests run against the built binary (make build), never the Vite dev server.
export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  workers: 1,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: 'retain-on-failure',
    // Unset uses Playwright's own browser; on this machine set PLAYWRIGHT_CHROMIUM_PATH=/usr/bin/chromium, which a WebKit run ignores.
    launchOptions: {
      executablePath:
        process.env.E2E_BROWSER === 'webkit'
          ? undefined
          : process.env.PLAYWRIGHT_CHROMIUM_PATH || undefined
    }
  },
  webServer: {
    // A fresh data directory every run, because the auth specs start from first-run setup.
    command: `rm -rf '${dataDir}' && exec ../bin/reefshepherd serve --dev --listen 127.0.0.1:${port} --data-dir '${dataDir}'`,
    url: `http://127.0.0.1:${port}/healthz`,
    reuseExistingServer: false,
    timeout: 30_000
  }
});
