import { describe, expect, it, vi } from 'vitest';
import { ApiError, createApi, isApiError } from './client.js';

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' }
  });
}

function setup(reply: () => Response | Promise<Response>, path = '/', search = '') {
  const fetchMock = vi.fn<typeof fetch>(async () => reply());
  const redirect = vi.fn<(to: string) => void>();
  const api = createApi({
    fetch: fetchMock,
    redirect,
    currentPath: () => path,
    currentSearch: () => search
  });
  return { api, fetchMock, redirect };
}

describe('requests', () => {
  it('prefixes the API path and appends the query without undefined entries', async () => {
    const { api, fetchMock } = setup(() => json(200, []));
    await api.get('/tanks', { query: { limit: 5, archived: false, name: undefined, q: null } });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/tanks?limit=5&archived=false');
  });

  it('extends a query string already in the path', async () => {
    const { api, fetchMock } = setup(() => json(200, []));
    await api.get('/tanks?sort=name', { query: { limit: 5 } });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/tanks?sort=name&limit=5');
  });

  it('sends JSON bodies with a JSON content type and the right verb', async () => {
    const { api, fetchMock } = setup(() => json(200, { id: 1 }));
    const created = await api.post<{ id: number }>('/tanks', { name: 'Reef' });
    await api.patch('/tanks/1', { name: 'Nano' });
    await api.put('/tanks/1', { name: 'Big' });
    await api.del('/tanks/1');

    expect(created).toEqual({ id: 1 });
    const [post, patch, put, del] = fetchMock.mock.calls.map((c) => c[1]!);
    expect([post.method, patch.method, put.method, del.method]).toEqual([
      'POST',
      'PATCH',
      'PUT',
      'DELETE'
    ]);
    expect(post.body).toBe('{"name":"Reef"}');
    expect((post.headers as Record<string, string>)['Content-Type']).toBe('application/json');
    expect(del.body).toBeUndefined();
    expect((del.headers as Record<string, string>)['Content-Type']).toBeUndefined();
  });

  it('sends a JSON body with DELETE when one is given', async () => {
    const { api, fetchMock } = setup(() => new Response(null, { status: 204 }));
    await api.del('/me/totp', { password: 'secret' });
    const init = fetchMock.mock.calls[0][1]!;
    expect(init.method).toBe('DELETE');
    expect(init.body).toBe('{"password":"secret"}');
    expect((init.headers as Record<string, string>)['Content-Type']).toBe('application/json');
  });

  it('passes the query options to DELETE after the body', async () => {
    const { api, fetchMock } = setup(() => new Response(null, { status: 204 }));
    await api.del('/tanks/1', undefined, { query: { force: true } });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/tanks/1?force=true');
    expect(fetchMock.mock.calls[0][1]!.body).toBeUndefined();
  });

  it('returns undefined for 204 replies', async () => {
    const { api } = setup(() => new Response(null, { status: 204 }));
    expect(await api.del('/tanks/1')).toBeUndefined();
  });

  it('reports an unreadable success body as bad_response', async () => {
    const { api } = setup(() => new Response('<html>', { status: 200 }));
    await expect(api.get('/me')).rejects.toMatchObject({ status: 200, code: 'bad_response' });
  });
});

describe('errors', () => {
  it('turns the server error shape into a typed ApiError', async () => {
    const { api } = setup(() =>
      json(422, { error: { code: 'validation', message: 'bad', fields: { value: 'required' } } })
    );
    const err = await api.post('/results', {}).catch((e: unknown) => e);
    expect(isApiError(err)).toBe(true);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({
      status: 422,
      code: 'validation',
      message: 'bad',
      fields: { value: 'required' }
    });
  });

  it('defaults fields to an empty object', async () => {
    const { api } = setup(() => json(404, { error: { code: 'not_found', message: 'gone' } }));
    await expect(api.get('/tanks/9')).rejects.toMatchObject({ code: 'not_found', fields: {} });
  });

  it('falls back to http_error when the body is not the error shape', async () => {
    const { api } = setup(() => new Response('<html>bad gateway</html>', { status: 502 }));
    await expect(api.get('/tanks')).rejects.toMatchObject({ status: 502, code: 'http_error' });
  });

  it('reads the Retry-After delay in seconds from a rate limit reply', async () => {
    const { api } = setup(
      () =>
        new Response(JSON.stringify({ error: { code: 'too_many_requests', message: 'slow' } }), {
          status: 429,
          headers: { 'Content-Type': 'application/json', 'Retry-After': '30' }
        })
    );
    await expect(api.post('/auth/login', {})).rejects.toMatchObject({
      status: 429,
      code: 'too_many_requests',
      retryAfter: 30
    });
  });

  it('leaves retryAfter undefined without a numeric Retry-After header', async () => {
    const { api } = setup(
      () =>
        new Response('nope', {
          status: 503,
          headers: { 'Retry-After': 'Wed, 21 Oct 2026 07:28:00 GMT' }
        })
    );
    const err = await api.get('/me').catch((e: unknown) => e);
    expect(err).toMatchObject({ code: 'http_error' });
    expect((err as ApiError).retryAfter).toBeUndefined();
  });

  it('maps a failed fetch to a network error with status 0', async () => {
    const { api } = setup(() => Promise.reject(new TypeError('Failed to fetch')));
    await expect(api.get('/tanks')).rejects.toMatchObject({ status: 0, code: 'network' });
  });

  it('lets an abort through unchanged so callers can ignore it', async () => {
    const abort = new DOMException('aborted', 'AbortError');
    const { api } = setup(() => Promise.reject(abort));
    await expect(api.get('/tanks')).rejects.toBe(abort);
  });
});

describe('redirects', () => {
  it('sends the browser to /login on 401 and still throws', async () => {
    const { api, redirect } = setup(() =>
      json(401, { error: { code: 'unauthorized', message: 'sign in' } })
    );
    await expect(api.get('/me')).rejects.toMatchObject({ status: 401 });
    expect(redirect).toHaveBeenCalledExactlyOnceWith('/login');
  });

  it('asks the login page to come back to the page that was open', async () => {
    const { api, redirect } = setup(
      () => json(401, { error: { code: 'unauthorized', message: 'sign in' } }),
      '/tanks/3/charts/5',
      '?range=30d'
    );
    await expect(api.get('/me')).rejects.toMatchObject({ status: 401 });
    expect(redirect).toHaveBeenCalledExactlyOnceWith(
      '/login?next=%2Ftanks%2F3%2Fcharts%2F5%3Frange%3D30d'
    );
  });

  it('does not redirect to /login when already on the login page', async () => {
    const { api, redirect } = setup(
      () => json(401, { error: { code: 'invalid_credentials', message: 'no' } }),
      '/login'
    );
    await expect(api.post('/auth/login', {})).rejects.toMatchObject({ status: 401 });
    expect(redirect).not.toHaveBeenCalled();
  });

  it('sends the browser to /setup when the server says setup is required', async () => {
    const { api, redirect } = setup(() =>
      json(409, { error: { code: 'setup_required', message: 'run setup' } })
    );
    await expect(api.get('/me')).rejects.toMatchObject({ code: 'setup_required' });
    expect(redirect).toHaveBeenCalledExactlyOnceWith('/setup');
  });

  it('prefers /setup over /login when the page is the login page', async () => {
    const { api, redirect } = setup(
      () => json(401, { error: { code: 'setup_required', message: 'run setup' } }),
      '/login'
    );
    await expect(api.get('/me')).rejects.toBeInstanceOf(ApiError);
    expect(redirect).toHaveBeenCalledExactlyOnceWith('/setup');
  });

  it('does not redirect for other failures', async () => {
    const { api, redirect } = setup(() =>
      json(403, { error: { code: 'forbidden', message: 'admins only' } })
    );
    await expect(api.get('/users')).rejects.toMatchObject({ status: 403 });
    expect(redirect).not.toHaveBeenCalled();
  });
});
