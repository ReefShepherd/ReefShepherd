import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import {
  createTank,
  ensureSignedIn,
  expectNoHorizontalScroll,
  expectOk,
  timeZone
} from './helpers.js';

// These steps share one server and a tank of their own, on whatever instance state the earlier specs left.
test.describe.configure({ mode: 'serial' });

let context: BrowserContext;
let page: Page;
let tankId = 0;
let phId = 0;
let nitrateId = 0;
// A reading at a fixed instant, so its displayed time can be checked against the instance timezone.
let flaggedAt = new Date();
const problems: string[] = [];

test.beforeAll(async ({ browser }, testInfo) => {
  context = await browser.newContext({
    baseURL: testInfo.project.use.baseURL,
    viewport: { width: 375, height: 812 },
    hasTouch: true
  });
  page = await context.newPage();
  page.on('console', (msg) => {
    if (msg.type() === 'error' && !msg.text().includes('404')) problems.push(msg.text());
  });
  await page.addInitScript(() => {
    document.addEventListener('securitypolicyviolation', (e) => {
      console.error(`CSP ${e.violatedDirective} ${e.blockedURI}`);
    });
  });

  const request = context.request;
  await ensureSignedIn(request);

  const tank = await createTank(request, { name: 'Chart tank', presets: ['ph', 'nitrate'] });
  tankId = tank.id;
  phId = tank.parameterIds.ph;
  nitrateId = tank.parameterIds.nitrate;

  // Nitrate only has an upper limit, which exercises a one-sided band.
  await expectOk(
    await request.put(`/api/v1/tanks/${tankId}/parameters/${nitrateId}`, {
      data: { ideal_low: null, ideal_high: 15 }
    })
  );

  const daysAgo = (days: number) => new Date(Date.now() - days * 86_400_000);
  flaggedAt = daysAgo(3);
  flaggedAt.setUTCMinutes(0, 0, 0);
  const readings: [Date, number, number, string][] = [
    [daysAgo(70), 8.1, 6, ''],
    [daysAgo(55), 8.2, 8, ''],
    [daysAgo(40), 8.0, 11, ''],
    [daysAgo(28), 8.3, 9, 'after water change'],
    [daysAgo(20), 8.6, 12, 'alkalinity overdosed'],
    [daysAgo(12), 8.2, 18, 'heavy feeding week'],
    [daysAgo(7), 8.1, 10, ''],
    [flaggedAt, 7.6, 7, 'skimmer was off']
  ];
  for (const [at, ph, nitrate, note] of readings) {
    await expectOk(
      await request.post(`/api/v1/tanks/${tankId}/results`, {
        data: {
          measured_at: at.toISOString(),
          results: [
            { parameter_id: phId, value: ph, note },
            { parameter_id: nitrateId, value: nitrate, note: '' }
          ]
        }
      })
    );
  }
});

test.afterAll(async () => {
  await context.close();
});

// Counts canvas pixels by color family: the ideal band is the only green on the plot, and flagged points are the only red.
async function plotPixels() {
  return page.locator('.plot canvas').evaluate((canvas: HTMLCanvasElement) => {
    const { data } = canvas.getContext('2d')!.getImageData(0, 0, canvas.width, canvas.height);
    let band = 0;
    let flagged = 0;
    for (let i = 0; i < data.length; i += 12) {
      const [r, g, b, a] = [data[i], data[i + 1], data[i + 2], data[i + 3]];
      if (a === 0) continue;
      if (g > r + 8 && g > b + 4) band++;
      if (r > g + 60 && r > b + 60) flagged++;
    }
    return { band, flagged };
  });
}

// Browsers and Node may format times with a plain, narrow or no-break space before AM/PM.
const spaces = new RegExp(`[${String.fromCharCode(0x202f)}${String.fromCharCode(0xa0)}]`, 'g');
function spaceNormalized(text: string) {
  return text.replace(spaces, ' ');
}

test('the charts index lists each parameter with a sparkline that links to its chart', async () => {
  await page.goto(`/tanks/${tankId}/charts`);
  await expect(page.getByRole('heading', { level: 1, name: 'Charts' })).toBeVisible();
  await expect(page.locator('.spark canvas')).toHaveCount(2);
  await expectNoHorizontalScroll(page);

  await page.getByRole('link', { name: /Nitrate/ }).click();
  await expect(page).toHaveURL(new RegExp(`/tanks/${tankId}/charts/${nitrateId}$`));
  await expect(page.getByRole('heading', { level: 1, name: 'Nitrate' })).toBeVisible();
});

test('a one-sided ideal range shades the plot and flags the high reading', async () => {
  await page.goto(`/tanks/${tankId}/charts/${nitrateId}`);
  await expect(page.getByText(/Ideal range: at most 15\.0\sppm/)).toBeVisible();
  await expect(page.locator('.plot canvas')).toBeVisible();
  await expect.poll(async () => (await plotPixels()).band).toBeGreaterThan(500);
  expect((await plotPixels()).flagged).toBeGreaterThan(10);
  await expect(page.getByRole('button', { name: /18\.0\sppm/ })).toBeVisible();
});

test('the pH chart shows the band, flags readings below the chart with their notes, and fits 375 px', async () => {
  await page.goto(`/tanks/${tankId}/charts/${phId}`);
  await expect(page.getByRole('heading', { level: 1, name: 'pH' })).toBeVisible();
  await expect(page.getByText('Ideal range: 7.9 to 8.4')).toBeVisible();
  await expect.poll(async () => (await plotPixels()).band).toBeGreaterThan(500);
  expect((await plotPixels()).flagged).toBeGreaterThan(10);

  const flagged = page.getByRole('heading', { name: 'Out of range readings' });
  await expect(flagged).toBeVisible();
  await expect(page.getByRole('button', { name: /alkalinity overdosed/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /skimmer was off/ })).toBeVisible();

  const box = await page.locator('.plot').boundingBox();
  expect(box!.width).toBeLessThanOrEqual(375);
  await expectNoHorizontalScroll(page);
});

test('selecting a flagged reading shows its value, its time in the instance timezone and its note', async () => {
  await page.getByRole('button', { name: /skimmer was off/ }).click();
  const detail = page.locator('.detail');
  await expect(detail).toContainText('7.6');
  await expect(detail).toContainText('Out of range');
  await expect(detail).toContainText('skimmer was off');
  const expected = new Intl.DateTimeFormat('en-US', {
    timeZone,
    dateStyle: 'medium',
    timeStyle: 'short'
  }).format(flaggedAt);
  expect(spaceNormalized((await detail.textContent()) ?? '')).toContain(spaceNormalized(expected));
});

test('tapping the plot selects the nearest reading', async () => {
  await page.goto(`/tanks/${tankId}/charts/${phId}`);
  await expect(page.getByText('Tap a point for its value, time and note.')).toBeVisible();
  await expect.poll(async () => (await plotPixels()).band).toBeGreaterThan(500);
  const box = (await page.locator('.plot .u-over').boundingBox())!;
  await page.touchscreen.tap(box.x + box.width * 0.5, box.y + box.height * 0.5);
  await expect(page.getByText('Tap a point for its value, time and note.')).toBeHidden();
  await expect(page.locator('.detail strong')).toHaveText(/^\d\.\d$/);
});

test('the range buttons reload the chart with the matching start', async () => {
  await page.goto(`/tanks/${tankId}/charts/${phId}`);
  await expect(page.getByRole('button', { name: '90 d' })).toHaveAttribute('aria-pressed', 'true');

  const thirty = page.waitForRequest((r) => /\/series\?from=/.test(r.url()));
  await page.getByRole('button', { name: '30 d' }).click();
  await thirty;
  await expect(page.getByRole('button', { name: '30 d' })).toHaveAttribute('aria-pressed', 'true');

  const all = page.waitForRequest((r) => /\/series$/.test(r.url()));
  await page.getByRole('button', { name: 'All' }).click();
  await all;
  await expect(page.getByRole('button', { name: 'All' })).toHaveAttribute('aria-pressed', 'true');
  await expect.poll(async () => (await plotPixels()).band).toBeGreaterThan(500);
});

for (const scheme of ['light', 'dark', 'oled', 'system-light', 'system-dark'] as const) {
  test(`the band and the flagged points are drawn in the ${scheme} theme`, async () => {
    const [theme, osScheme] = scheme.startsWith('system')
      ? ['system', scheme.slice(7)]
      : [scheme, 'light'];
    // The account's saved theme is applied once the session loads, so it is set before the page opens.
    await expectOk(await context.request.patch('/api/v1/me', { data: { theme } }));
    await page.emulateMedia({ colorScheme: osScheme as 'light' | 'dark' });
    await page.goto(`/tanks/${tankId}/charts/${phId}`);
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
    await expect(page.locator('.plot canvas')).toBeVisible();
    await expect.poll(async () => (await plotPixels()).band).toBeGreaterThan(500);
    expect((await plotPixels()).flagged).toBeGreaterThan(10);
    await expectNoHorizontalScroll(page);
  });
}

test('switching the theme while a chart is open repaints it', async () => {
  await expectOk(await context.request.patch('/api/v1/me', { data: { theme: 'light' } }));
  await page.goto(`/tanks/${tankId}/charts/${phId}`);
  await page.waitForLoadState('networkidle');
  await expect.poll(async () => (await plotPixels()).band).toBeGreaterThan(500);
  const before = await page
    .locator('.plot canvas')
    .evaluate((c: HTMLCanvasElement) => c.toDataURL());
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'oled';
  });
  await expect
    .poll(() => page.locator('.plot canvas').evaluate((c: HTMLCanvasElement) => c.toDataURL()))
    .not.toBe(before);
  expect((await plotPixels()).band).toBeGreaterThan(500);
});

test('no chart page raised a CSP violation or a console error', async () => {
  expect(problems).toEqual([]);
});
