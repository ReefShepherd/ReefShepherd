import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import {
  createTank,
  createTask,
  daysAgo,
  ensureSignedIn,
  expectNoHorizontalScroll,
  expectOk
} from './helpers.js';

// These steps share one server and one tank, each building on the task the one before it made.
test.describe.configure({ mode: 'serial' });

let context: BrowserContext;
let page: Page;
let tankId = 0;
let taskId = 0;

test.beforeAll(async ({ browser }, testInfo) => {
  context = await browser.newContext({ baseURL: testInfo.project.use.baseURL });
  page = await context.newPage();
  await ensureSignedIn(context.request);
  tankId = (await createTank(context.request, { name: 'Maintenance tank', presets: [] })).id;
});

test.afterAll(async () => {
  // Tasks left behind would raise reminders in the notification specs.
  const tasks = (await (
    await context.request.get('/api/v1/maintenance/tasks?status=all')
  ).json()) as {
    items: { id: number }[];
  };
  for (const task of tasks.items) {
    await expectOk(await context.request.delete(`/api/v1/maintenance/tasks/${task.id}`));
  }
  await context.close();
});

function taskItem(page: Page, name: string) {
  return page.locator('li.item').filter({ has: page.getByRole('link', { name, exact: true }) });
}

test('a task from the Water change template with a last done date is overdue', async () => {
  await page.goto(`/tanks/${tankId}/maintenance`);
  await expect(
    page.getByRole('heading', { level: 1, name: /Maintenance.*Maintenance tank/ })
  ).toBeVisible();
  await expect(page.getByText('There are no tasks yet.', { exact: false })).toHaveCount(0);

  await page.getByRole('button', { name: 'Add task' }).click();
  await page.getByLabel('Start from a template').selectOption({ label: 'Water change' });
  await expect(page.getByLabel('Name', { exact: true })).toHaveValue('Water change');
  await page.getByLabel('Last done on (optional)').fill(daysAgo(8));
  await page.getByRole('button', { name: 'Add task' }).last().click();

  await expect(page.getByText('Added Water change.')).toBeVisible();
  await expect(page.getByRole('button', { name: /^Due \(1\)/ })).toBeVisible();
  const item = taskItem(page, 'Water change');
  await expect(item).toBeVisible();
  await expect(item.getByText('Overdue')).toBeVisible();
  await expect(item.getByText(/Last done/)).toBeVisible();

  const tasks = (await (
    await context.request.get(`/api/v1/maintenance/tasks?tank_id=${tankId}&status=all`)
  ).json()) as { items: { id: number; name: string; overdue: boolean }[] };
  const water = tasks.items.find((t) => t.name === 'Water change');
  expect(water?.overdue).toBe(true);
  taskId = water?.id ?? 0;
});

test('marking the task done moves it from due to upcoming and into its history', async () => {
  await page.goto(`/tanks/${tankId}/maintenance`);
  const item = taskItem(page, 'Water change');
  await item.getByRole('button', { name: 'Done Water change' }).click();
  await page.getByLabel('Note (optional)').fill('first run');
  await page.getByRole('button', { name: 'Mark done' }).click();

  await expect(page.getByText('Marked Water change done.')).toBeVisible();
  await expect(page.getByRole('button', { name: /^Due \(0\)/ })).toBeVisible();
  await page.getByRole('button', { name: /^Upcoming \(1\)/ }).click();
  await expect(taskItem(page, 'Water change')).toBeVisible();
  await expect(taskItem(page, 'Water change').getByText('Overdue')).toHaveCount(0);

  await taskItem(page, 'Water change').getByRole('link', { name: 'History Water change' }).click();
  await expect(page).toHaveURL(new RegExp(`/maintenance/${taskId}$`));
  await expect(page.getByRole('heading', { level: 2, name: 'History' })).toBeVisible();
  const entries = page.locator('li.card').filter({ has: page.getByText('Done', { exact: true }) });
  await expect(entries).toHaveCount(2);
  await expect(entries.filter({ hasText: 'first run' })).toHaveCount(1);
});

test('a history entry can be edited and deleted', async () => {
  await page.goto(`/maintenance/${taskId}`);
  const entries = page.locator('li.card').filter({ has: page.getByText('Done', { exact: true }) });
  await expect(entries).toHaveCount(2);

  const recent = entries.filter({ hasText: 'first run' });
  await recent.getByRole('button', { name: /^Edit/ }).click();
  await page.getByLabel('Note (optional)').fill('edited run');
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(entries.filter({ hasText: 'edited run' })).toHaveCount(1);
  await expect(entries.filter({ hasText: 'first run' })).toHaveCount(0);

  const older = entries.filter({ hasNotText: 'edited run' });
  await older.getByRole('button', { name: /^Delete the entry/ }).click();
  await page.getByRole('button', { name: 'Delete', exact: true }).click();
  await expect(entries).toHaveCount(1);
  await expect(entries.filter({ hasText: 'edited run' })).toHaveCount(1);
});

test('skipping a task adds a skipped entry to its history', async () => {
  await page.goto(`/maintenance/${taskId}`);
  await page.getByRole('button', { name: 'Skip Water change' }).click();
  await page.getByRole('button', { name: 'Skip it' }).click();

  await expect(page.locator('li.card').getByText('Skipped', { exact: true })).toBeVisible();
  await expect(page.getByText(/Last skipped/)).toBeVisible();
  await expect(page.locator('li.card').getByText('Done', { exact: true })).toHaveCount(1);
});

test('a whole-system task is listed on a tank page under its own heading', async () => {
  await page.goto(`/tanks/${tankId}/maintenance`);
  await page.getByRole('button', { name: 'Add task' }).click();
  await page
    .getByLabel('Start from a template')
    .selectOption({ label: 'Replace RO/DI cartridges' });
  await expect(page.getByLabel('Belongs to').locator('option:checked')).toHaveText('Whole system');
  await page.getByRole('button', { name: 'Add task' }).last().click();
  await expect(page.getByText('Added Replace RO/DI cartridges.')).toBeVisible();

  await page.getByRole('button', { name: /^All \(/ }).click();
  const system = page.getByRole('region', { name: 'Whole system' });
  await expect(
    system.getByRole('link', { name: 'Replace RO/DI cartridges', exact: true })
  ).toBeVisible();
  const own = page.getByRole('region', { name: 'This tank' });
  await expect(own.getByRole('link', { name: 'Water change', exact: true })).toBeVisible();
});

test('the card on a new tank dashboard lists due tasks and marks them done', async () => {
  const request = context.request;
  const fresh = await createTank(request, { name: 'Fresh tank', presets: [] });
  await createTask(request, {
    name: 'Rinse the filter',
    tankId: fresh.id,
    everyDays: 1,
    lastDoneDaysAgo: 3,
    notify: false
  });

  const dashboard = await context.newPage();
  await dashboard.addInitScript((id) => {
    try {
      localStorage.setItem('current_tank', String(id));
    } catch {
      /* storage unavailable */
    }
  }, fresh.id);
  await dashboard.goto('/');
  const card = dashboard
    .locator('.dash-slot')
    .filter({ has: dashboard.getByRole('heading', { name: 'Maintenance due', exact: true }) });
  await expect(card).toBeVisible();
  await expect(card.locator('.name', { hasText: 'Rinse the filter' })).toBeVisible();
  const rinse = card.getByRole('listitem').filter({ hasText: 'Rinse the filter' });
  await expect(rinse.getByText('Overdue')).toBeVisible();
  // A whole-system task shows on every tank's card, and another tank's task does not.
  await expect(
    card.getByRole('listitem').filter({ hasText: 'Replace RO/DI cartridges' })
  ).toBeVisible();
  await expect(card.getByText('Water change')).toHaveCount(0);

  await card.getByRole('button', { name: 'Done Rinse the filter' }).click();
  await expect(rinse).toHaveCount(0);
  await dashboard.close();
});

test('the maintenance pages fit a 375 px screen', async () => {
  await page.setViewportSize({ width: 375, height: 812 });
  for (const path of [`/tanks/${tankId}/maintenance`, `/maintenance/${taskId}`, '/maintenance']) {
    await page.goto(path);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
    await expect(page.getByText('Loading')).toHaveCount(0);
    await expectNoHorizontalScroll(page);
  }

  await page.goto(`/tanks/${tankId}/maintenance`);
  await page.getByRole('button', { name: 'Add task' }).click();
  await expect(page.getByRole('heading', { name: 'New task' })).toBeVisible();
  await expectNoHorizontalScroll(page);
});
