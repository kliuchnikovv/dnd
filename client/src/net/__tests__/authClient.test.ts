import { googleLogin, refresh, me, AuthError } from '../authClient';

function mockFetchOnce(status: number, body: unknown) {
  (global as any).fetch = jest.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  });
}

describe('authClient', () => {
  it('googleLogin maps server session', async () => {
    mockFetchOnce(200, {
      access_token: 'a', refresh_token: 'r', expires_at: 1700000000,
      user: { id: 'u1', email: 'a@b.c', name: 'Ann', picture: 'p' },
    });
    const s = await googleLogin('id-token');
    expect(s.accessToken).toBe('a');
    expect(s.refreshToken).toBe('r');
    expect(s.expiresAt).toBe(1700000000);
    expect(s.user.email).toBe('a@b.c');
    const [, opts] = (global.fetch as jest.Mock).mock.calls[0];
    expect(JSON.parse(opts.body)).toEqual({ id_token: 'id-token' });
  });

  it('refresh maps session', async () => {
    mockFetchOnce(200, {
      access_token: 'a2', refresh_token: 'r2', expires_at: 1700000100,
      user: { id: 'u1', email: 'a@b.c', name: 'Ann', picture: 'p' },
    });
    const s = await refresh('r');
    expect(s.accessToken).toBe('a2');
    expect(s.refreshToken).toBe('r2');
  });

  it('me returns the user', async () => {
    mockFetchOnce(200, { id: 'u1', email: 'a@b.c', name: 'Ann', picture: 'p' });
    const u = await me('a');
    expect(u.id).toBe('u1');
    const [, opts] = (global.fetch as jest.Mock).mock.calls[0];
    expect(opts.headers.Authorization).toBe('Bearer a');
  });

  it('throws AuthError with status on 401', async () => {
    mockFetchOnce(401, { error: 'вход отклонён' });
    await expect(googleLogin('bad')).rejects.toMatchObject({ status: 401 });
    await expect(googleLogin('bad')).rejects.toBeInstanceOf(AuthError);
  });
});
