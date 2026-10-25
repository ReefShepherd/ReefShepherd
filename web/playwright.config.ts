import { defineConfig } from '@playwright/test';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const port = 8486;

// End-to-end tests run against the built binary (make build), never the Vite dev server.
export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    // Unset uses Playwright's own browser; on this machine set PLAYWRIGHT_CHROMIUM_PATH=/usr/bin/chromium.
    launchOptions: { executablePath: process.env.PLAYWRIGHT_CHROMIUM_PATH || undefined }
  },
  webServer: {
    command: `../bin/reefshepherd serve --dev --listen 127.0.0.1:${port} --data-dir ${join(tmpdir(), 'reefshepherd-e2e')}`,
    url: `http://127.0.0.1:${port}/healthz`,
    reuseExistingServer: false,
    timeout: 30_000
  }
});
