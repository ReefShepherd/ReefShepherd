import {
  expect,
  test,
  type Browser,
  type BrowserContext,
  type Locator,
  type Page
} from '@playwright/test';
import { createTank, ensureSignedIn, expectNoHorizontalScroll, expectOk } from './helpers.js';

// These steps share one server and a tank of their own, on whatever instance state the earlier specs left.
test.describe.configure({ mode: 'serial' });

const mateName = 'dashmate';
const matePassword = 'another long passphrase 2026';

let owner: BrowserContext;
let mate: BrowserContext;
let page: Page;
let matePage: Page;
let tankId = 0;
const problems: string[] = [];

const defaultOrder = ['Tests due', 'pH', 'Alkalinity', 'Nitrate'];

// Pins the current tank before any page script runs, so the dashboard opens this test's tank.
async function newContext(browser: Browser, baseURL: string) {
  const context = await browser.newContext({
    baseURL,
    viewport: { width: 375, height: 812 },
    hasTouch: true
  });
  await context.addInitScript((id) => {
    try {
      localStorage.setItem('current_tank', String(id));
    } catch {
      /* storage unavailable */
    }
  }, tankId);
  return context;
}

test.beforeAll(async ({ browser }, testInfo) => {
  const baseURL = String(testInfo.project.use.baseURL);
  // The first context only exists to sign in and create data; its tank id is not known yet.
  const bootstrap = await browser.newContext({ baseURL });
  await ensureSignedIn(bootstrap.request);
  const tank = await createTank(bootstrap.request, {
    name: 'Dashboard tank',
    presets: ['ph', 'alkalinity', 'nitrate']
  });
  tankId = tank.id;
  const idOf = (key: string): number => tank.parameterIds[key];
  // pH is inside its range and nitrate is above its limit; alkalinity has no reading at all.
  await expectOk(
    await bootstrap.request.post(`/api/v1/tanks/${tankId}/results`, {
      data: {
        results: [
          { parameter_id: idOf('ph'), value: 8.2 },
          { parameter_id: idOf('nitrate'), value: 80 }
        ]
      }
    })
  );

  // A second member, who signs in from a separate browser context.
  await expectOk(
    await bootstrap.request.post('/api/v1/users', {
      data: { username: mateName, password: matePassword, role: 'member' }
    })
  );
  const storage = await bootstrap.storageState();
  await bootstrap.close();

  owner = await newContext(browser, baseURL);
  await owner.addCookies(storage.cookies);
  page = await owner.newPage();

  mate = await newContext(browser, baseURL);
  const login = await mate.request.post('/api/v1/auth/login', {
    data: { username: mateName, password: matePassword }
  });
  expect(login.ok(), await login.text()).toBe(true);
  matePage = await mate.newPage();

  for (const p of [page, matePage]) {
    p.on('console', (msg) => {
      if (msg.type() === 'error' && !msg.text().includes('404')) problems.push(msg.text());
    });
  }
});

test.afterAll(async () => {
  await owner.close();
  await mate.close();
});

// The heading of each card, in the order the cards appear in the page.
async function titles(p: Page): Promise<string[]> {
  return p
    .locator('.dash-slot')
    .evaluateAll((slots) =>
      slots.map((slot) => slot.querySelector('h3')?.textContent?.trim() ?? '')
    );
}

function slot(p: Page, title: string): Locator {
  return p
    .locator('.dash-slot')
    .filter({ has: p.getByRole('heading', { name: title, exact: true }) });
}

async function openDashboard(p: Page) {
  await p.goto('/');
  await expect(p.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible();
  await expect(p.locator('.dash-slot').first()).toBeVisible();
}

async function startEditing(p: Page) {
  await p.getByRole('button', { name: 'Edit dashboard' }).click();
  await expect(p.getByRole('button', { name: 'Save layout' })).toBeVisible();
}

async function save(p: Page) {
  const saved = p.waitForResponse(
    (r) => r.url().includes(`/tanks/${tankId}/dashboard`) && r.request().method() === 'PUT'
  );
  await p.getByRole('button', { name: 'Save layout' }).click();
  expect((await saved).ok()).toBe(true);
  await expect(p.getByRole('button', { name: 'Edit dashboard' })).toBeVisible();
}

test('a new dashboard shows the default cards with values, ranges and a tests-due card', async () => {
  await openDashboard(page);
  expect(await titles(page)).toEqual(defaultOrder);

  const ph = slot(page, 'pH');
  await expect(ph).toContainText('8.2');
  await expect(ph).toContainText('In range');
  await expect(ph).toContainText(/Tested/);

  // The out-of-range state is spelled out in text, not only shown in color.
  const nitrate = slot(page, 'Nitrate');
  await expect(nitrate).toContainText('Out of range');
  await expect(slot(page, 'Alkalinity')).toContainText('No tests logged yet');

  await expect(page.getByRole('link', { name: 'pH', exact: true })).toHaveAttribute(
    'href',
    new RegExp(`/tanks/${tankId}/charts/\\d+$`)
  );
  await expectNoHorizontalScroll(page);
});

test('the drag library is not requested until edit mode opens', async () => {
  const scripts: string[] = [];
  page.on('request', (r) => {
    if (r.resourceType() === 'script') scripts.push(r.url());
  });
  await openDashboard(page);
  const before = scripts.length;
  await startEditing(page);
  await expect.poll(() => scripts.length).toBeGreaterThan(before);
});

test('editing adds a chart card, widens a card, removes one, and the layout survives a reload', async () => {
  await openDashboard(page);
  await startEditing(page);

  await page.getByLabel('Add a card').selectOption({ label: 'pH chart' });
  await page.getByRole('button', { name: 'Add card' }).click();

  const wide = page.getByRole('button', { name: 'Wide Nitrate' });
  await expect(wide).toHaveAttribute('aria-pressed', 'false');
  await wide.click();
  await expect(wide).toHaveAttribute('aria-pressed', 'true');

  await page.getByRole('button', { name: 'Remove Alkalinity' }).click();
  await save(page);

  const expected = ['Tests due', 'pH', 'Nitrate', 'pH'];
  expect(await titles(page)).toEqual(expected);

  await page.reload();
  await expect(page.locator('.dash-slot').first()).toBeVisible();
  expect(await titles(page)).toEqual(expected);
  await expect(slot(page, 'Nitrate')).toHaveClass(/wide/);
  await expect(page.locator('.dash-slot canvas').first()).toBeVisible();
  await expect(page.getByRole('button', { name: 'Edit dashboard' })).toBeVisible();
});

test('the up and down buttons reorder cards and keep focus on the pressed button', async () => {
  await openDashboard(page);
  await startEditing(page);

  await page.getByRole('button', { name: 'Move Nitrate up' }).click();
  expect((await titles(page)).slice(0, 3)).toEqual(['Tests due', 'Nitrate', 'pH']);
  await expect(page.getByRole('button', { name: 'Move Nitrate up' })).toBeFocused();

  await page.getByRole('button', { name: 'Move Nitrate down' }).click();
  expect((await titles(page)).slice(0, 3)).toEqual(['Tests due', 'pH', 'Nitrate']);

  // The first card cannot move up and the last cannot move down.
  await expect(page.getByRole('button', { name: 'Move Tests due up' })).toBeDisabled();
  await page.getByRole('button', { name: 'Cancel' }).click();
  await expect(page.getByRole('button', { name: 'Edit dashboard' })).toBeVisible();
});

test('a card can be picked up with Space, moved with the arrow keys and dropped, then saved', async () => {
  await openDashboard(page);
  const before = await titles(page);
  await startEditing(page);

  await page.getByRole('button', { name: 'Drag Tests due' }).focus();
  await page.keyboard.press('Space');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Space');

  const after = (await titles(page)).slice();
  expect(after[0]).toBe(before[1]);
  expect(after[1]).toBe(before[0]);

  await save(page);
  await page.reload();
  await expect(page.locator('.dash-slot').first()).toBeVisible();
  expect((await titles(page))[1]).toBe(before[0]);
});

test('cancel discards edits and reset restores the default cards', async () => {
  await openDashboard(page);
  const kept = await titles(page);

  await startEditing(page);
  await page.getByRole('button', { name: 'Remove Nitrate' }).click();
  await page.getByRole('button', { name: 'Cancel' }).click();
  expect(await titles(page)).toEqual(kept);

  await startEditing(page);
  await page.getByRole('button', { name: 'Reset to default' }).click();
  await page.getByRole('button', { name: 'Reset', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Edit dashboard' })).toBeVisible();
  expect(await titles(page)).toEqual(defaultOrder);
});

test('the last card cannot be removed', async () => {
  await openDashboard(page);
  await startEditing(page);
  for (const name of ['Alkalinity', 'Nitrate', 'pH']) {
    await page.getByRole('button', { name: `Remove ${name}` }).click();
  }
  await expect(page.locator('.dash-slot')).toHaveCount(1);
  await expect(page.getByRole('button', { name: 'Remove Tests due' })).toBeDisabled();
  await expect(page.getByText('A dashboard needs at least one card.')).toBeVisible();
  await page.getByRole('button', { name: 'Cancel' }).click();
});

test('wide and normal cards fit a 375 px screen in view and edit mode', async () => {
  await openDashboard(page);
  await expectNoHorizontalScroll(page);
  const grid = await page.locator('.dash-grid').boundingBox();
  for (const box of await Promise.all(
    (await page.locator('.dash-slot').all()).map((s) => s.boundingBox())
  )) {
    expect(box!.width).toBeLessThanOrEqual(grid!.width + 1);
  }

  await startEditing(page);
  await expectNoHorizontalScroll(page);
  // Every edit control is a comfortable touch target.
  for (const button of await page.locator('.bar button, .bar .grip').all()) {
    const box = await button.boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(43);
    expect(box!.width).toBeGreaterThanOrEqual(43);
  }
  await page.getByRole('button', { name: 'Cancel' }).click();
});

test('two users keep independent layouts for the same tank', async () => {
  // The first user trims the dashboard down.
  await openDashboard(page);
  await startEditing(page);
  await page.getByRole('button', { name: 'Remove Alkalinity' }).click();
  await page.getByRole('button', { name: 'Remove Nitrate' }).click();
  await save(page);
  expect(await titles(page)).toEqual(['Tests due', 'pH']);

  // The second user still has the default cards, and their own edit does not touch the first.
  await openDashboard(matePage);
  expect(await titles(matePage)).toEqual(defaultOrder);
  await startEditing(matePage);
  await matePage.getByRole('button', { name: 'Remove Tests due' }).click();
  await matePage.getByRole('button', { name: 'Wide pH' }).click();
  await save(matePage);
  expect(await titles(matePage)).toEqual(['pH', 'Alkalinity', 'Nitrate']);

  await page.reload();
  await expect(page.locator('.dash-slot').first()).toBeVisible();
  expect(await titles(page)).toEqual(['Tests due', 'pH']);
  await expect(slot(page, 'pH')).not.toHaveClass(/wide/);

  await matePage.reload();
  await expect(matePage.locator('.dash-slot').first()).toBeVisible();
  expect(await titles(matePage)).toEqual(['pH', 'Alkalinity', 'Nitrate']);
  await expect(slot(matePage, 'pH')).toHaveClass(/wide/);
});

test('no console errors were raised', () => {
  expect(problems).toEqual([]);
});
