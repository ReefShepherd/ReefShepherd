import { expect, type APIRequestContext, type APIResponse, type Page } from '@playwright/test';

/** The administrator every spec signs in as, and the instance setup that creates it. */
export const username = 'reefkeeper';
export const password = 'correct horse battery staple';
export const instanceName = 'Garage reef';
export const timeZone = 'Europe/Berlin';

export async function expectOk(response: APIResponse): Promise<void> {
  expect(response.ok(), await response.text()).toBe(true);
}

/** Leaves the request context signed in as the administrator, running first-run setup when the instance has none. */
export async function ensureSignedIn(request: APIRequestContext): Promise<void> {
  const config = await (await request.get('/api/v1/auth/config')).json();
  if (config.setup_required) {
    await expectOk(
      await request.post('/api/v1/setup', {
        data: { username, password, instance_name: instanceName, timezone: timeZone }
      })
    );
    return;
  }
  if ((await request.get('/api/v1/me')).ok()) return;
  const login = await request.post('/api/v1/auth/login', { data: { username, password } });
  await expectOk(login);
  expect((await login.json()).mfa_required).toBeFalsy();
}

export interface CreatedTank {
  id: number;
  /** The parameter id of each preset the tank was created with, by preset key. */
  parameterIds: Record<string, number>;
}

/** Creates a tank through the API; the request context must already be signed in. */
export async function createTank(
  request: APIRequestContext,
  options: { name: string; presets: string[]; volume?: number }
): Promise<CreatedTank> {
  const created = await request.post('/api/v1/tanks', {
    data: { name: options.name, volume: options.volume ?? 100, presets: options.presets }
  });
  await expectOk(created);
  const id = (await created.json()).id as number;

  const list = await (await request.get(`/api/v1/tanks/${id}/parameters`)).json();
  const parameterIds: Record<string, number> = {};
  for (const p of list.parameters as { parameter_id: number; preset_key: string | null }[]) {
    if (p.preset_key) parameterIds[p.preset_key] = p.parameter_id;
  }
  return { id, parameterIds };
}

/** Archives every active tank, so the instance looks like one that has never had a tank. */
export async function archiveAllTanks(request: APIRequestContext): Promise<void> {
  const { items: tanks } = (await (await request.get('/api/v1/tanks')).json()) as {
    items: { id: number }[];
  };
  for (const tank of tanks) await expectOk(await request.post(`/api/v1/tanks/${tank.id}/archive`));
}

export async function expectNoHorizontalScroll(page: Page): Promise<void> {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth
  );
  expect(overflow).toBeLessThanOrEqual(0);
}

/** The calendar date `days` days before now in the instance time zone, as YYYY-MM-DD. */
export function daysAgo(days: number): string {
  const at = new Date(Date.now() - days * 86_400_000);
  return new Intl.DateTimeFormat('en-CA', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  }).format(at);
}

export interface CreatedTask {
  id: number;
  name: string;
}

/** Creates a task that repeats a number of days after it was last done; a tank of null makes it a whole-system task. */
export async function createTask(
  request: APIRequestContext,
  options: {
    name: string;
    tankId: number | null;
    everyDays?: number;
    lastDoneDaysAgo?: number;
    notify?: boolean;
  }
): Promise<CreatedTask> {
  const created = await request.post('/api/v1/maintenance/tasks', {
    data: {
      tank_id: options.tankId,
      name: options.name,
      notify: options.notify ?? true,
      schedule: {
        kind: 'after_last',
        interval_days: options.everyDays ?? 7,
        weekdays: null,
        interval_weeks: 1,
        month_day: null,
        anchor_date: null,
        due_time: '09:00'
      },
      ...(options.lastDoneDaysAgo === undefined
        ? {}
        : { last_done_on: daysAgo(options.lastDoneDaysAgo) })
    }
  });
  await expectOk(created);
  return { id: (await created.json()).id as number, name: options.name };
}

export interface CreatedChannel {
  id: number;
  name: string;
}

/** Creates an enabled webhook channel that receives every event. */
export async function createWebhookChannel(
  request: APIRequestContext,
  options: { name: string; url: string }
): Promise<CreatedChannel> {
  const created = await request.post('/api/v1/notification-channels', {
    data: {
      name: options.name,
      kind: 'webhook',
      config: { url: options.url },
      events: ['maintenance_due', 'tests_due', 'out_of_range', 'backup_failed']
    }
  });
  await expectOk(created);
  return { id: (await created.json()).id as number, name: options.name };
}

/** Marks every notification read for the signed-in user. */
export async function markAllNotificationsRead(request: APIRequestContext): Promise<void> {
  await expectOk(await request.post('/api/v1/notifications/read', { data: { all: true } }));
}

/** Deletes every notification channel, so no earlier spec's destination receives later messages. */
export async function deleteAllChannels(request: APIRequestContext): Promise<void> {
  const { items } = (await (await request.get('/api/v1/notification-channels')).json()) as {
    items: { id: number }[];
  };
  for (const channel of items) {
    await expectOk(await request.delete(`/api/v1/notification-channels/${channel.id}`));
  }
}
