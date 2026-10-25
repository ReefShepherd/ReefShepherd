import { expect, test, type BrowserContext, type Locator, type Page } from '@playwright/test';
import { archiveAllTanks, ensureSignedIn, expectNoHorizontalScroll } from './helpers.js';

// These steps share one server, one signed-in phone-sized session and one tank, so they run in order and each builds on the last.
test.describe.configure({ mode: 'serial' });

let context: BrowserContext;
let page: Page;

test.beforeAll(async ({ browser }, testInfo) => {
  context = await browser.newContext({
    baseURL: testInfo.project.use.baseURL,
    viewport: { width: 375, height: 812 }
  });
  page = await context.newPage();

  // Earlier specs leave tanks behind, so the instance is put back to one without any active tank.
  await ensureSignedIn(context.request);
  await archiveAllTanks(context.request);
});

test.afterAll(async () => {
  await context.close();
});

const history = () => page.getByRole('region', { name: 'Results history' });
const resultItem = (name: string): Locator =>
  history().getByRole('listitem').filter({ hasText: name });

test('a signed-in user with no tanks lands on the first-tank form', async () => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/tanks\/new$/);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Create your first tank');
  await expect(page.getByRole('link', { name: 'Create a tank' })).toBeVisible();
  await expectNoHorizontalScroll(page);

  // The dashboard address leads back to the form while there is no tank.
  await page.goto('/');
  await expect(page).toHaveURL(/\/tanks\/new$/);
});

test('creating a tank with the preset parameters makes it the current tank', async () => {
  await expect(page.getByRole('checkbox', { name: /^pH/ })).toBeChecked();
  await expect(page.getByRole('checkbox', { name: /^Calcium/ })).toBeChecked();

  // A name is required before anything is sent.
  await page.getByRole('button', { name: 'Create tank' }).click();
  await expect(page.getByText('This field is required.')).toBeVisible();
  await expect(page.getByLabel('Name')).toBeFocused();

  await page.getByLabel('Name').fill('Display');
  await page.getByLabel('Volume').fill('300,5');
  await page.getByRole('button', { name: 'Create tank' }).click();

  await expect(page).toHaveURL(/\/$/);
  await expect(page.locator('#tank-switcher option:checked')).toHaveText('Display');
  await expectNoHorizontalScroll(page);
});

test('tests for several parameters are logged in one form, with a decimal comma and a note', async () => {
  await page.getByRole('link', { name: 'Log tests' }).click();
  await expect(page).toHaveURL(/\/tanks\/\d+\/tests$/);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Log tests · Display');
  await expectNoHorizontalScroll(page);

  // Text that is not a number is flagged and nothing is sent.
  await page.getByLabel('pH value').fill('abc');
  await page.getByRole('button', { name: 'Save results' }).click();
  await expect(page.getByText('Enter a number such as 7.9 or 7,9.')).toBeVisible();
  await expect(page.getByLabel('pH value')).toBeFocused();

  await page.getByLabel('pH value').fill('7,9');
  await page.getByRole('button', { name: 'Add a note pH', exact: true }).click();
  await page.getByLabel('pH note').fill('after water change');
  await page.getByLabel('Calcium value').fill('420');
  await page.getByLabel('Nitrate value').fill('60');
  await page.getByRole('button', { name: 'Save results' }).click();

  await expect(page.getByText('Saved 3 results.')).toBeVisible();
  await expect(page.getByLabel('pH value')).toHaveValue('');

  await expect(resultItem('pH')).toContainText('7.9');
  await expect(resultItem('pH')).toContainText('after water change');
  await expect(resultItem('Calcium')).toContainText('420');
  await expect(resultItem('Calcium')).not.toContainText('Out of range');
  await expect(resultItem('Nitrate')).toContainText('Out of range');
  await expectNoHorizontalScroll(page);
});

test('the history can be filtered to one parameter', async () => {
  await page.getByLabel('Show').selectOption({ label: 'Calcium' });
  await expect(history().getByRole('listitem')).toHaveCount(1);
  await expect(resultItem('Calcium')).toBeVisible();

  await page.getByLabel('Show').selectOption({ label: 'All parameters' });
  await expect(history().getByRole('listitem')).toHaveCount(3);
});

test('a result can be corrected in place and the out-of-range mark follows the value', async () => {
  await resultItem('Nitrate').getByRole('button', { name: 'Edit' }).click();
  await resultItem('Nitrate').getByLabel('Nitrate value').fill('12,5');
  await resultItem('Nitrate').getByLabel('Note').fill('re-tested');
  await resultItem('Nitrate').getByRole('button', { name: 'Save changes' }).click();

  await expect(resultItem('Nitrate')).toContainText('12.5');
  await expect(resultItem('Nitrate')).toContainText('re-tested');
  await expect(resultItem('Nitrate')).not.toContainText('Out of range');
  await expectNoHorizontalScroll(page);
});

test('a result can be deleted after a confirmation', async () => {
  await resultItem('Calcium').getByRole('button', { name: 'Delete' }).click();
  await resultItem('Calcium').getByRole('button', { name: 'Cancel' }).click();
  await expect(resultItem('Calcium')).toBeVisible();

  await resultItem('Calcium').getByRole('button', { name: 'Delete' }).click();
  await resultItem('Calcium').getByRole('button', { name: 'Delete' }).click();
  await expect(history().getByRole('listitem')).toHaveCount(2);
  await expect(resultItem('Calcium')).toHaveCount(0);

  // The corrections survive a reload.
  await page.reload();
  await expect(history().getByRole('listitem')).toHaveCount(2);
  await expect(resultItem('Nitrate')).toContainText('12.5');
});

test('the tank list and settings show the new tank', async () => {
  await page.getByRole('link', { name: 'Tanks', exact: true }).click();
  await expect(page).toHaveURL(/\/tanks$/);
  await expect(page.getByRole('heading', { name: 'Display' })).toBeVisible();
  await expectNoHorizontalScroll(page);

  await page.getByRole('link', { name: /^Settings Display/ }).click();
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Display settings');
  await expect(page.getByRole('heading', { name: 'Parameters' })).toBeVisible();
  await expectNoHorizontalScroll(page);
});

test('a parameter moved with the keyboard keeps focus on the button that was pressed', async () => {
  const down = page.getByRole('button', { name: 'Move pH down' });
  await down.focus();
  await page.keyboard.press('Enter');
  await expect(down).toBeFocused();
});
