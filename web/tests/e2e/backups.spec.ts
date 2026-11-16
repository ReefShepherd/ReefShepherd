import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import { ensureSignedIn, expectOk } from './helpers.js';

// These steps share one server and the backup list it keeps, each building on the one before it.
test.describe.configure({ mode: 'serial' });

let context: BrowserContext;
let page: Page;
let backupName = '';

test.beforeAll(async ({ browser }, testInfo) => {
  context = await browser.newContext({ baseURL: testInfo.project.use.baseURL });
  page = await context.newPage();
  await ensureSignedIn(context.request);
});

test.afterAll(async () => {
  // The settings go back to their defaults so no later spec depends on this one.
  await expectOk(
    await context.request.patch('/api/v1/settings', {
      data: { backup_enabled: true, backup_time: '03:30', backup_keep: 14 }
    })
  );
  await context.close();
});

test('back up now adds a backup to the list', async () => {
  await page.goto('/settings/backups');
  await expect(page.getByRole('heading', { name: 'Saved backups' })).toBeVisible();

  // The first scheduled backup can land in the same second as a click, which the page reports as a collision to retry.
  const notice = page.getByText(/^Backup reefshepherd-.+\.db created\.$/);
  await expect(async () => {
    await page.getByRole('button', { name: 'Back up now' }).click();
    await expect(notice).toBeVisible({ timeout: 2_000 });
  }).toPass({ timeout: 20_000 });
  backupName = ((await notice.textContent()) ?? '').match(/Backup (\S+) created/)?.[1] ?? '';
  expect(backupName).not.toBe('');

  await expect(page.locator('li').filter({ hasText: backupName })).toBeVisible();
  const listed = (await (await context.request.get('/api/v1/backups')).json()) as {
    items: { name: string; size_bytes: number }[];
  };
  const made = listed.items.find((b) => b.name === backupName);
  expect(made?.size_bytes).toBeGreaterThan(0);
});

test('the backup downloads as an attachment', async () => {
  const link = page.getByRole('link', { name: `Download ${backupName}` });
  await expect(link).toBeVisible();
  const href = await link.getAttribute('href');
  expect(href).toContain(encodeURIComponent(backupName));

  const response = await context.request.get(href ?? '');
  expect(response.status()).toBe(200);
  expect(response.headers()['content-disposition']).toMatch(/^attachment/);
  expect(response.headers()['content-disposition']).toContain(backupName);
  expect(response.headers()['content-type']).toBe('application/vnd.sqlite3');
  const body = await response.body();
  // Every SQLite file starts with the same 16 bytes.
  expect(body.subarray(0, 15).toString('latin1')).toBe('SQLite format 3');
});

test('a backup can be deleted', async () => {
  const row = page.locator('li').filter({ hasText: backupName });
  await row.getByRole('button', { name: `Delete ${backupName}` }).click();
  await page.getByRole('button', { name: 'Delete backup' }).click();

  await expect(page.getByText(`Backup ${backupName} deleted.`)).toBeVisible();
  await expect(page.locator('li').filter({ hasText: backupName })).toHaveCount(0);
  const gone = await context.request.get(`/api/v1/backups/${backupName}/download`);
  expect(gone.status()).toBe(404);
});

test('the automatic backup settings persist', async () => {
  await page.goto('/settings/backups');
  const enabled = page.getByLabel('Back up automatically every day');
  await expect(enabled).toBeChecked();
  await enabled.uncheck();
  await page.getByLabel('Backup time').fill('04:15');
  await page.getByLabel('Scheduled backups to keep').fill('7');
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByText('Backup settings saved.')).toBeVisible();

  await page.reload();
  await expect(page.getByLabel('Back up automatically every day')).not.toBeChecked();
  await expect(page.getByLabel('Backup time')).toHaveValue('04:15');
  await expect(page.getByLabel('Scheduled backups to keep')).toHaveValue('7');

  const settings = (await (await context.request.get('/api/v1/settings')).json()) as {
    backup_enabled: boolean;
    backup_time: string;
    backup_keep: number;
  };
  expect(settings).toMatchObject({ backup_enabled: false, backup_time: '04:15', backup_keep: 7 });
});
