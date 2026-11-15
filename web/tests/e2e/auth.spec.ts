import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { expectNoHorizontalScroll, password, username } from './helpers.js';
import { totpCode } from './totp.js';

// These steps share one server and one data directory, so they run in order and each builds on the last.
test.describe.configure({ mode: 'serial' });

// A signed-in user with no tanks is sent on from the home page to the first-tank page.
const home = /\/(tanks\/new)?$/;

let secret = '';
let recoveryCodes: string[] = [];

// One phone-sized browser session is shared by all the steps, so cookies and the saved theme carry over.
let context: BrowserContext;
let page: Page;

test.beforeAll(async ({ browser }, testInfo) => {
  context = await browser.newContext({
    baseURL: testInfo.project.use.baseURL,
    viewport: { width: 375, height: 812 }
  });
  page = await context.newPage();
});

test.afterAll(async () => {
  await context.close();
});

async function signOutEverywhere(page: Page) {
  // Let the open page finish its requests first, or a late 401 starts its own redirect and aborts the navigation below.
  await page.waitForLoadState('networkidle');
  await context.clearCookies();
  await page.goto('/login');
}

async function signInWithPassword(page: Page) {
  await page.getByLabel('Username').fill(username);
  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Sign in' }).click();
}

test('first-run setup creates the instance and signs the administrator in', async () => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/setup$/);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Set up ReefShepherd');
  await expectNoHorizontalScroll(page);

  await page.getByLabel('Instance name').fill('Garage reef');
  await page.getByLabel('Timezone').fill('Europe/Berlin');
  await page.getByRole('radio', { name: 'Dark' }).check();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await page.getByRole('button', { name: 'Next' }).click();

  await expect(page.getByLabel('Salinity')).toHaveValue('sg');
  await page.getByLabel('Temperature').selectOption('f');
  await page.getByRole('button', { name: 'Next' }).click();

  // A short password is refused before anything is sent.
  await page.getByLabel('Username', { exact: true }).fill(username);
  await page.getByLabel('Password', { exact: true }).fill('short');
  await page.getByLabel('Repeat the password').fill('short');
  await page.getByRole('button', { name: 'Finish setup' }).click();
  await expect(page.getByText('Use at least 8 characters.')).toBeVisible();
  await expect(page.getByLabel('Password', { exact: true })).toBeFocused();

  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByLabel('Repeat the password').fill(password);
  await page.getByRole('button', { name: 'Finish setup' }).click();

  await expect(page).toHaveURL(home);
  await expect(page.getByRole('navigation', { name: 'Main' })).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');

  // Setup is closed once it has run, and the sign-in page then passes a signed-in user on.
  await page.goto('/setup');
  await expect(page).toHaveURL(home);
});

test('the chosen units and name are kept in the instance settings', async () => {
  await page.goto('/settings/instance');
  await expect(page.getByLabel('Instance name')).toHaveValue('Garage reef');
  await expect(page.getByLabel('Timezone')).toHaveValue('Europe/Berlin');
  await expect(page.getByLabel('Temperature')).toHaveValue('f');
  await expect(page.getByLabel('Salinity')).toHaveValue('sg');
  await expectNoHorizontalScroll(page);
});

test('two-step login is enrolled with a QR code and recovery codes', async () => {
  await page.goto('/settings/account');
  await page.getByRole('button', { name: 'Set up two-step login' }).click();

  await expect(page.getByRole('img', { name: 'QR code for your authenticator app' })).toBeVisible();
  secret = (await page.locator('p.hint .mono').innerText()).trim();
  expect(secret).toMatch(/^[A-Z2-7]{16,}$/);

  await page.getByLabel('Authentication code').fill('000000');
  await page.getByRole('button', { name: 'Turn on' }).click();
  await expect(page.getByText('That code is not valid.')).toBeVisible();

  await page.getByLabel('Authentication code').fill(totpCode(secret));
  await page.getByRole('button', { name: 'Turn on' }).click();

  const list = page.getByRole('list', { name: 'Recovery codes' });
  await expect(list.getByRole('listitem')).toHaveCount(10);
  recoveryCodes = await list.getByRole('listitem').allInnerTexts();
  expect(recoveryCodes[0]).toMatch(/^[A-Z2-9]{5}-[A-Z2-9]{5}$/);

  const downloadPromise = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Download as text file' }).click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe('reefshepherd-recovery-codes.txt');
  const text = readFileSync(await download.path(), 'utf8');
  for (const code of recoveryCodes) expect(text).toContain(code);

  await page.getByRole('button', { name: 'I have saved these codes' }).click();
  await expect(list).toBeHidden();
  await expect(page.getByText(/Two-step login is on/)).toBeVisible();
  await expectNoHorizontalScroll(page);
});

test('signing in asks for the authenticator code and works from the keyboard', async () => {
  await signOutEverywhere(page);
  await expectNoHorizontalScroll(page);

  await page.getByLabel('Username').fill(username);
  await page.getByLabel('Password', { exact: true }).fill('the wrong password entirely');
  await page.keyboard.press('Enter');
  await expect(page.getByText('The username or password is not correct.')).toBeVisible();

  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.keyboard.press('Enter');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Two-step login');

  // The enrolment used the current step, so the next one is the first code the server has not seen.
  await expect(page.getByLabel('Authentication code')).toBeFocused();
  await page.keyboard.type(totpCode(secret, 1));
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(home);
  await expect(page.getByRole('navigation', { name: 'Main' })).toBeVisible();
});

test('a recovery code signs in once', async () => {
  const [code] = recoveryCodes;

  await signOutEverywhere(page);
  await signInWithPassword(page);
  await page.getByRole('button', { name: 'Use a recovery code instead' }).click();
  await page.getByLabel('Recovery code').fill(code.toLowerCase());
  await page.getByRole('button', { name: 'Verify' }).click();
  await expect(page).toHaveURL(home);

  await signOutEverywhere(page);
  await signInWithPassword(page);
  await page.getByRole('button', { name: 'Use a recovery code instead' }).click();
  await page.getByLabel('Recovery code').fill(code);
  await page.getByRole('button', { name: 'Verify' }).click();
  await expect(page.getByText('That code is not valid.').first()).toBeVisible();
  await expect(page).toHaveURL(/\/login$/);
});

test('the session list shows this device and two-step login can be turned off', async () => {
  await signOutEverywhere(page);
  await signInWithPassword(page);
  await page.getByRole('button', { name: 'Use a recovery code instead' }).click();
  await page.getByLabel('Recovery code').fill(recoveryCodes[1]);
  await page.getByRole('button', { name: 'Verify' }).click();
  await expect(page).toHaveURL(home);

  await page.goto('/settings/account');
  await expect(page.getByText('This device')).toBeVisible();

  await page.getByLabel('Password', { exact: true }).fill('not my password');
  await page.getByRole('button', { name: 'Turn off', exact: true }).click();
  await expect(page.getByText('That password is not correct.')).toBeVisible();

  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Turn off', exact: true }).click();
  await expect(page.getByText('Two-step login is off.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Set up two-step login' })).toBeVisible();
});

test('turning login off needs the password, then nobody has to sign in', async () => {
  await page.goto('/settings/instance');
  await page.getByRole('radio', { name: /No login/ }).check();
  await expect(
    page.getByText(/anyone who can reach this address can see and change everything/)
  ).toBeVisible();

  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByText('This field is required.')).toBeVisible();

  await page.getByLabel('Your password').fill('not my password');
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByText('That password is not correct.')).toBeVisible();

  await page.getByLabel('Your password').fill(password);
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByText('Settings saved.')).toBeVisible();
  await expect(page.getByText(/Login is turned off/)).toBeVisible();

  await context.clearCookies();
  await page.goto('/');
  await expect(page.getByRole('navigation', { name: 'Main' })).toBeVisible();
  await expect(page).toHaveURL(home);

  await page.goto('/login');
  await expect(page).toHaveURL(home);
});

test('turning login back on sends the browser to sign in', async () => {
  await page.goto('/settings/instance');
  await page.getByRole('radio', { name: /Require a login/ }).check();
  await page.getByRole('button', { name: 'Save changes' }).click();
  // The page the browser was on is remembered, so signing in brings it back there.
  await expect(page).toHaveURL(/\/login\?next=%2Fsettings%2Finstance$/);

  await signInWithPassword(page);
  await expect(page).toHaveURL(/\/settings\/instance$/);
});

test('signing in returns to the page that asked for it, and a signed-in user skips the form', async () => {
  await page.goto('/login');
  await expect(page).toHaveURL(home);

  await signOutEverywhere(page);
  await page.goto('/settings/account');
  await expect(page).toHaveURL(/\/login\?next=%2Fsettings%2Faccount$/);
  await signInWithPassword(page);
  await expect(page).toHaveURL(/\/settings\/account$/);

  // A return address that leaves the site is ignored.
  await signOutEverywhere(page);
  await page.goto('/login?next=https%3A%2F%2Fexample.com%2F');
  await signInWithPassword(page);
  await expect(page).toHaveURL(home);
});
