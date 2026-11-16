import { createServer, type Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import {
  createTank,
  createTask,
  createWebhookChannel,
  deleteAllChannels,
  ensureSignedIn,
  expectOk,
  instanceName,
  markAllNotificationsRead
} from './helpers.js';

// These steps share one server, one receiver and one channel, each building on the one before it.
test.describe.configure({ mode: 'serial' });

// The scheduler runs every 5 seconds in the dev server, so a wait covers a few ticks plus the delivery.
const tickWait = 45_000;

interface Received {
  path: string;
  contentType: string | undefined;
  json: Record<string, unknown> | null;
}

let context: BrowserContext;
let page: Page;
let receiver: Server;
let receiverUrl = '';
const received: Received[] = [];
let tankId = 0;
const tankName = 'Reminder tank';

function requestsTitled(title: string): Received[] {
  return received.filter((r) => r.json?.title === title);
}

function unreadCount(): Promise<number> {
  return context.request
    .get('/api/v1/notifications/unread-count')
    .then((res) => res.json())
    .then((body: { count: number }) => body.count);
}

function channelCard(name: string) {
  return page.getByRole('region', { name, exact: true });
}

test.beforeAll(async ({ browser }, testInfo) => {
  // The receiver lives in this process, so it is not a second app instance; paths under /fail answer 500.
  receiver = createServer((req, res) => {
    const chunks: Buffer[] = [];
    req.on('data', (chunk: Buffer) => chunks.push(chunk));
    req.on('end', () => {
      let json: Record<string, unknown> | null = null;
      try {
        json = JSON.parse(Buffer.concat(chunks).toString('utf8')) as Record<string, unknown>;
      } catch {
        /* not JSON */
      }
      received.push({
        path: req.url ?? '',
        contentType: req.headers['content-type'],
        json
      });
      const failing = (req.url ?? '').startsWith('/fail');
      res.statusCode = failing ? 500 : 200;
      res.end(failing ? 'receiver unavailable' : 'ok');
    });
  });
  await new Promise<void>((resolve) => receiver.listen(0, '127.0.0.1', resolve));
  receiverUrl = `http://127.0.0.1:${(receiver.address() as AddressInfo).port}`;

  context = await browser.newContext({ baseURL: testInfo.project.use.baseURL });
  page = await context.newPage();
  await ensureSignedIn(context.request);
  await deleteAllChannels(context.request);
  tankId = (await createTank(context.request, { name: tankName, presets: [] })).id;
});

test.afterAll(async () => {
  await deleteAllChannels(context.request);
  const tasks = (await (
    await context.request.get('/api/v1/maintenance/tasks?status=all')
  ).json()) as {
    items: { id: number }[];
  };
  for (const task of tasks.items) {
    await expectOk(await context.request.delete(`/api/v1/maintenance/tasks/${task.id}`));
  }
  await context.close();
  receiver.closeAllConnections();
  await new Promise<void>((resolve) => receiver.close(() => resolve()));
});

test('an administrator adds a webhook channel and its test message reaches the receiver', async () => {
  await page.goto('/settings/notifications');
  await expect(page.getByText('No channels yet.')).toBeVisible();

  await page.getByRole('button', { name: 'Add channel' }).click();
  await page.getByLabel('Kind').selectOption({ label: 'Webhook' });
  await page.getByLabel('Name', { exact: true }).fill('Receiver');
  await page.getByLabel('URL', { exact: true }).fill(`${receiverUrl}/hook`);
  await page.getByRole('button', { name: 'Save channel' }).click();

  const card = channelCard('Receiver');
  await expect(card).toBeVisible();
  await expect(card.getByText('Nothing sent yet')).toBeVisible();

  await card.getByRole('button', { name: 'Send test' }).click();
  await expect(card.getByText('The test message was sent.')).toBeVisible();

  await expect.poll(() => requestsTitled(`Test notification from ${instanceName}`).length).toBe(1);
  const [request] = requestsTitled(`Test notification from ${instanceName}`);
  expect(request.path).toBe('/hook');
  expect(request.contentType).toContain('application/json');
  expect(request.json).toMatchObject({
    version: 1,
    event: 'test',
    severity: 'info',
    tank: null
  });

  // The stored URL is a secret, so the API never gives it back.
  const channels = (await (await context.request.get('/api/v1/notification-channels')).json()) as {
    items: { name: string; config: { url: string } }[];
  };
  expect(channels.items.find((c) => c.name === 'Receiver')?.config.url).toBe('********');
});

test('a due task reaches the inbox and the receiver, and the bell follows the inbox', async () => {
  await markAllNotificationsRead(context.request);
  await page.goto('/');
  const bell = page.locator('a.bell');
  await expect(bell).toHaveAttribute('aria-label', 'Notifications');

  const title = `Rinse the skimmer is due on ${tankName}`;
  await createTask(context.request, {
    name: 'Rinse the skimmer',
    tankId,
    everyDays: 1,
    lastDoneDaysAgo: 3
  });

  await expect.poll(() => requestsTitled(title).length, { timeout: tickWait }).toBe(1);
  expect(requestsTitled(title)[0].json).toMatchObject({
    version: 1,
    event: 'maintenance_due',
    tank: { id: tankId, name: tankName }
  });

  // The bell polls once a minute, so a reload stands in for opening the app again.
  await expect(async () => {
    await page.reload();
    await expect(bell).toHaveAttribute('aria-label', /^Notifications, \d+ unread$/);
  }).toPass({ timeout: tickWait });
  expect(await unreadCount()).toBeGreaterThan(0);

  await page.goto('/notifications');
  await expect(page.getByRole('link', { name: title })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Mark all read' })).toBeEnabled();
  await page.getByRole('button', { name: 'Mark all read' }).click();

  await expect(page.getByRole('button', { name: 'Mark all read' })).toBeDisabled();
  await expect(bell).toHaveAttribute('aria-label', 'Notifications');
  expect(await unreadCount()).toBe(0);
  await page.reload();
  await expect(page.getByRole('link', { name: title })).toBeVisible();
  await expect(page.locator('a.bell')).toHaveAttribute('aria-label', 'Notifications');
});

test('a receiver that answers 500 shows the error on the channel after a real delivery', async () => {
  const failing = await createWebhookChannel(context.request, {
    name: 'Broken receiver',
    url: `${receiverUrl}/fail`
  });

  // A test send reports its failure to the person but does not count as an attempt on the channel.
  const testSend = await context.request.post(`/api/v1/notification-channels/${failing.id}/test`);
  expect(await testSend.json()).toMatchObject({ sent: false });
  const afterTest = (await (await context.request.get('/api/v1/notification-channels')).json()) as {
    items: { id: number; last_error: string }[];
  };
  expect(afterTest.items.find((c) => c.id === failing.id)?.last_error).toBe('');

  const title = `Check the return pump is due on ${tankName}`;
  await createTask(context.request, {
    name: 'Check the return pump',
    tankId,
    everyDays: 1,
    lastDoneDaysAgo: 3
  });

  await expect
    .poll(() => received.some((r) => r.path === '/fail' && r.json?.title === title), {
      timeout: tickWait
    })
    .toBe(true);
  // The healthy channel still gets the same message.
  await expect.poll(() => requestsTitled(title).some((r) => r.path === '/hook')).toBe(true);

  await expect(async () => {
    await page.goto('/settings/notifications');
    await expect(
      channelCard('Broken receiver').getByText(/The last attempt failed.*HTTP 500/)
    ).toBeVisible({ timeout: 2_000 });
  }).toPass({ timeout: tickWait });
  await expect(channelCard('Receiver').getByText(/The last attempt failed/)).toHaveCount(0);
});
