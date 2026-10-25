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
  const tanks = (await (await request.get('/api/v1/tanks')).json()) as { id: number }[];
  for (const tank of tanks) await expectOk(await request.post(`/api/v1/tanks/${tank.id}/archive`));
}

export async function expectNoHorizontalScroll(page: Page): Promise<void> {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth
  );
  expect(overflow).toBeLessThanOrEqual(0);
}
