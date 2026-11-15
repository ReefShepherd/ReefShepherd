import { expect, test, type BrowserContext, type Locator, type Page } from '@playwright/test';
import {
  archiveAllTanks,
  createTank,
  ensureSignedIn,
  expectNoHorizontalScroll
} from './helpers.js';

// Set E2E_BROWSER=webkit (and leave PLAYWRIGHT_CHROMIUM_PATH unset) to run the same measurements in WebKit.
test.use({
  browserName: (process.env.E2E_BROWSER as 'chromium' | 'webkit' | undefined) ?? 'chromium'
});

// These steps share one server, one signed-in phone-sized session and one tank, so they run in order.
test.describe.configure({ mode: 'serial' });

let context: BrowserContext;
let page: Page;
let tankId: number;

test.beforeAll(async ({ browser }, testInfo) => {
  context = await browser.newContext({
    baseURL: testInfo.project.use.baseURL,
    viewport: { width: 375, height: 812 }
  });
  page = await context.newPage();

  await ensureSignedIn(context.request);
  await archiveAllTanks(context.request);
  tankId = (await createTank(context.request, { name: 'Date check', presets: ['ph'] })).id;
});

test.afterAll(async () => {
  await context.close();
});

interface Layout {
  left: number;
  right: number;
  boxLeft: number;
  boxRight: number;
  textAlign: string;
  valueAlign: string;
  minWidth: string;
  boxSizing: string;
}

/** Measures an input against the card (or the page column) that holds it, plus the styles that decide how its value is placed. */
async function layoutOf(input: Locator): Promise<Layout> {
  return input.evaluate((el) => {
    const box = el.closest('.card') ?? el.closest('main') ?? document.body;
    const rect = el.getBoundingClientRect();
    const outer = box.getBoundingClientRect();
    const boxStyle = getComputedStyle(box);
    const style = getComputedStyle(el);
    return {
      left: rect.left,
      right: rect.right,
      boxLeft: outer.left + parseFloat(boxStyle.borderLeftWidth) + parseFloat(boxStyle.paddingLeft),
      boxRight:
        outer.right - parseFloat(boxStyle.borderRightWidth) - parseFloat(boxStyle.paddingRight),
      textAlign: style.textAlign,
      valueAlign: getComputedStyle(el, '::-webkit-date-and-time-value').textAlign,
      minWidth: style.minWidth,
      boxSizing: style.boxSizing
    };
  });
}

async function expectFitsLeftAligned(input: Locator): Promise<void> {
  await expect(input).toBeVisible();
  const layout = await layoutOf(input);
  expect(layout.right, 'right edge stays inside the card').toBeLessThanOrEqual(
    layout.boxRight + 0.5
  );
  expect(layout.left, 'left edge stays inside the card').toBeGreaterThanOrEqual(
    layout.boxLeft - 0.5
  );
  expect(layout.textAlign).toBe('left');
  // The pseudo-element only exists where the engine styles date values through it.
  if (layout.valueAlign) expect(layout.valueAlign).toBe('left');
  expect(layout.minWidth).toBe('0px');
  expect(layout.boxSizing).toBe('border-box');
}

test('the tank form start date fits the card and is left aligned', async () => {
  await page.goto('/tanks/new');
  await expectFitsLeftAligned(page.getByLabel('Start date'));
  await expectNoHorizontalScroll(page);
});

test('the Log tests date and time fits the page column and is left aligned', async () => {
  await page.goto(`/tanks/${tankId}/tests`);
  await expectFitsLeftAligned(page.getByLabel('Date and time'));
  await expectNoHorizontalScroll(page);
});

test('the results edit row date and time fits its card and is left aligned', async () => {
  await page.getByLabel('pH value').fill('8,1');
  await page.getByRole('button', { name: 'Save results' }).click();
  const item = page
    .getByRole('region', { name: 'Results history' })
    .getByRole('listitem')
    .filter({ hasText: 'pH' });
  await item.getByRole('button', { name: 'Edit' }).click();
  await expectFitsLeftAligned(item.getByLabel('Date and time'));
  await expectNoHorizontalScroll(page);
});

test('the schedule editor start date and due time fit their card and are left aligned', async () => {
  await page.goto(`/tanks/${tankId}/settings`);
  await page.getByRole('button', { name: 'Edit pH', exact: true }).click();
  await page.getByLabel('Repeats').selectOption('every_n_days');
  const schedule = page.getByRole('group', { name: 'Schedule' });
  await expectFitsLeftAligned(schedule.getByLabel('Start date'));
  await expectFitsLeftAligned(schedule.getByLabel('Due at'));
  await expectNoHorizontalScroll(page);
});
