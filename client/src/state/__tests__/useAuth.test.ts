import { createAuthStore } from '../useAuth';
import { memoryTokenStore } from '../../auth/tokenStore';
import { fakeGoogleSignIn } from '../../auth/googleSignIn';
import type { Session, AuthUser } from '../../net/authClient';

const user: AuthUser = { id: 'u1', email: 'a@b.c', name: 'Ann', picture: 'p' };
const sess: Session = { accessToken: 'a', refreshToken: 'r', expiresAt: 111, user };

function deps(over: Partial<any> = {}) {
  return {
    client: {
      googleLogin: jest.fn(async (_id: string) => sess),
      refresh: jest.fn(async (_r: string) => ({ ...sess, accessToken: 'a2', refreshToken: 'r2' })),
      me: jest.fn(async (_a: string) => user),
      ...over.client,
    },
    tokens: over.tokens ?? memoryTokenStore(),
    google: over.google ?? fakeGoogleSignIn('id-token'),
  };
}

describe('useAuth store', () => {
  it('login: google → server → stores tokens → signedIn', async () => {
    const d = deps();
    const store = createAuthStore(d);
    await store.getState().login();
    expect(store.getState().status).toBe('signedIn');
    expect(store.getState().user?.email).toBe('a@b.c');
    expect(await d.tokens.load()).toMatchObject({ accessToken: 'a', refreshToken: 'r' });
  });

  it('restore: no tokens → signedOut', async () => {
    const store = createAuthStore(deps());
    await store.getState().restore();
    expect(store.getState().status).toBe('signedOut');
  });

  it('restore: stored tokens → me() ok → signedIn', async () => {
    const tokens = memoryTokenStore();
    await tokens.save({ accessToken: 'a', refreshToken: 'r', expiresAt: 111 });
    const store = createAuthStore(deps({ tokens }));
    await store.getState().restore();
    expect(store.getState().status).toBe('signedIn');
    expect(store.getState().user?.id).toBe('u1');
  });

  it('restore: me() 401 → refresh() → signedIn with new tokens', async () => {
    const tokens = memoryTokenStore();
    await tokens.save({ accessToken: 'stale', refreshToken: 'r', expiresAt: 1 });
    const meErr = Object.assign(new Error('x'), { status: 401 });
    const d = deps({ tokens, client: { me: jest.fn().mockRejectedValueOnce(meErr).mockResolvedValue(user) } });
    const store = createAuthStore(d);
    await store.getState().restore();
    expect(d.client.refresh).toHaveBeenCalledWith('r');
    expect(store.getState().status).toBe('signedIn');
    expect(await tokens.load()).toMatchObject({ accessToken: 'a2' });
  });

  it('restore: tokens.load() rejects → signedOut (not stuck on loading)', async () => {
    const tokens = memoryTokenStore();
    tokens.load = jest.fn().mockRejectedValue(new Error('secure-store unavailable'));
    const store = createAuthStore(deps({ tokens }));
    await store.getState().restore();
    expect(store.getState().status).toBe('signedOut');
  });

  it('logout: clears tokens → signedOut', async () => {
    const d = deps();
    const store = createAuthStore(d);
    await store.getState().login();
    await store.getState().logout();
    expect(store.getState().status).toBe('signedOut');
    expect(await d.tokens.load()).toBeNull();
  });

  it('refresh: success keeps signedIn with new tokens', async () => {
    const tokens = memoryTokenStore();
    await tokens.save({ accessToken: 'a', refreshToken: 'r', expiresAt: 111 });
    const d = deps({ tokens });
    const store = createAuthStore(d);
    await store.getState().restore();
    await store.getState().refresh();
    expect(store.getState().status).toBe('signedIn');
    expect(await tokens.load()).toMatchObject({ accessToken: 'a2', refreshToken: 'r2' });
  });

  it('refresh: failure → signedOut, tokens cleared', async () => {
    const tokens = memoryTokenStore();
    await tokens.save({ accessToken: 'a', refreshToken: 'r', expiresAt: 111 });
    const d = deps({
      tokens,
      client: { refresh: jest.fn(async () => { throw new Error('refresh failed'); }) },
    });
    const store = createAuthStore(d);
    await store.getState().restore();
    await store.getState().refresh();
    expect(store.getState().status).toBe('signedOut');
    expect(await tokens.load()).toBeNull();
  });

  it('login(google?): explicit google param overrides deps.google', async () => {
    const d = deps();
    const overrideGoogle = fakeGoogleSignIn('override-token');
    const store = createAuthStore(d);
    await store.getState().login(overrideGoogle);
    expect(store.getState().status).toBe('signedIn');
    expect(d.client.googleLogin).toHaveBeenCalledWith('override-token');
  });
});
