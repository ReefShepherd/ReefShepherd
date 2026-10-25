import { expect, test, type ConsoleMessage } from '@playwright/test';

// /login is a bare route, so these checks do not depend on whether the instance has been set up.

// The sign-in page asks /me whether anyone is already signed in, and a signed-out browser is answered with 401.
function isSignedOutProbe(msg: ConsoleMessage): boolean {
  return (
    msg.text().includes('401') &&
    new URL(msg.location().url, 'http://localhost').pathname === '/api/v1/me'
  );
}

test('the built app loads with no CSP violations or console errors', async ({ page }) => {
  const problems: string[] = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error' && !isSignedOutProbe(msg)) problems.push(msg.text());
  });
  await page.addInitScript(() => {
    document.addEventListener('securitypolicyviolation', (e) => {
      (window as unknown as { csp: string[] }).csp ??= [];
      (window as unknown as { csp: string[] }).csp.push(`${e.violatedDirective} ${e.blockedURI}`);
    });
  });

  await page.goto('/login');
  await expect(page.locator('h1')).toBeVisible();

  const violations = await page.evaluate(() => (window as unknown as { csp?: string[] }).csp ?? []);
  expect(violations).toEqual([]);
  expect(problems).toEqual([]);
});

test('the stored theme is applied before first paint', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('theme', 'oled'));
  await page.goto('/login');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'oled');
  const background = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  expect(background).toBe('rgb(0, 0, 0)');
});

test('the stored theme is already painted before any app script has loaded', async ({ page }) => {
  // With every external script blocked, only the inline script in app.html can have applied the theme.
  await page.route('**/*.js', (route) => route.abort());
  await page.addInitScript(() => localStorage.setItem('theme', 'oled'));
  await page.goto('/login');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'oled');
  const background = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  expect(background).toBe('rgb(0, 0, 0)');
});

test('the manifest is served with its own media type and its icons exist', async ({ request }) => {
  const res = await request.get('/manifest.webmanifest');
  expect(res.headers()['content-type']).toContain('application/manifest+json');
  const manifest = (await res.json()) as { icons: { src: string }[] };
  for (const icon of manifest.icons) {
    expect((await request.get(icon.src)).ok(), icon.src).toBe(true);
  }
});
