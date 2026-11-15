import { paraglideVitePlugin } from '@inlang/paraglide-js';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { execSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { defineConfig } from 'vite';
import { extractThemeInitScript } from './src/lib/theme/constants.js';

// The no-flash theme script lives in app.html; hash it so the CSP allows exactly that code.
const appHtml = readFileSync(new URL('./src/app.html', import.meta.url), 'utf8');
const themeScript = extractThemeInitScript(appHtml);
if (themeScript === undefined) throw new Error('src/app.html has no theme init script');
const themeHash = `sha256-${createHash('sha256').update(themeScript).digest('base64')}` as const;

// SvelteKit's route announcer carries a fixed inline style attribute that style-src 'self' would block.
const announcerStyleHash = 'sha256-S8qMpvofolR8Mpjy4kQvEm7m1q8clzU4dfDH0AmvZjo=';

// Rebuilding the same commit must produce identical files, so the build version is not a timestamp.
function buildVersion(): string {
  if (process.env.REEFSHEPHERD_WEB_VERSION) return process.env.REEFSHEPHERD_WEB_VERSION;
  try {
    return execSync('git rev-parse --short HEAD', { stdio: ['ignore', 'pipe', 'ignore'] })
      .toString()
      .trim();
  } catch {
    return 'dev';
  }
}

export default defineConfig({
  plugins: [
    paraglideVitePlugin({
      project: './project.inlang',
      outdir: './src/lib/paraglide',
      strategy: ['baseLocale'],
      disableAsyncLocalStorage: true
    }),
    sveltekit({
      adapter: adapter({
        pages: '../internal/webui/dist',
        assets: '../internal/webui/dist',
        fallback: '200.html',
        precompress: true,
        strict: true
      }),
      paths: { relative: false },
      version: { name: buildVersion() },
      csp: {
        mode: 'hash',
        directives: {
          'default-src': ['none'],
          'script-src': ['self', themeHash],
          'style-src': ['self'],
          'style-src-attr': ['unsafe-hashes', announcerStyleHash],
          'img-src': ['self', 'data:'],
          'font-src': ['self'],
          'connect-src': ['self'],
          'manifest-src': ['self'],
          'base-uri': ['self'],
          'form-action': ['self']
        }
      }
    })
  ],
  server: {
    port: 5173,
    strictPort: true,
    proxy: { '/api': { target: 'http://127.0.0.1:8485', changeOrigin: false } }
  }
});
