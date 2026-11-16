// Sums the gzip size of every script the built app shell loads up front and fails above the budget.
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

const limit = 53_684;
const dist = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'internal', 'webui', 'dist');

let html;
try {
  html = readFileSync(join(dist, '200.html'), 'utf8');
} catch {
  console.error('200.html not found; run npm run build first');
  process.exit(2);
}

const found = new Set();
for (const tag of html.matchAll(/<(?:script|link)\b[^>]*>/g)) {
  const isScript = tag[0].startsWith('<script');
  const isPreload = /\brel=["']modulepreload["']/.test(tag[0]);
  if (!isScript && !isPreload) continue;
  const url = tag[0].match(isScript ? /\bsrc=["']([^"']+)["']/ : /\bhref=["']([^"']+)["']/);
  if (url) found.add(url[1]);
}
for (const call of html.matchAll(/\bimport\(\s*["']([^"']+)["']\s*\)/g)) found.add(call[1]);

let total = 0;
for (const url of [...found].sort()) {
  if (!url.startsWith('/') || !url.endsWith('.js')) {
    console.error(`cannot measure ${url}; expected an absolute path to a .js file`);
    process.exit(2);
  }
  total += gzipSync(readFileSync(join(dist, url)), { level: 9 }).length;
}
if (found.size === 0) {
  console.error('200.html references no scripts, so there is nothing to measure');
  process.exit(2);
}

console.log(`initial JS: ${total} bytes gzip (limit ${limit})`);
if (total > limit) {
  console.error(`over the limit by ${total - limit} bytes`);
  process.exit(1);
}
