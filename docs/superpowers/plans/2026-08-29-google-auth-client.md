# Google-авторизация: клиент (B). План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Экраны входа через Google в RN-клиенте: первый вход = регистрация; access+refresh хранятся безопасно; оболочка закрыта гейтом до входа. Токен готов для будущего NetSource (сам NetSource — вне области).

**Architecture:** Логика вынесена за интерфейсы и тестируется на fakes: `authClient` (fetch к серверным /auth/*), `tokenStore` (expo-secure-store), `googleSignIn` (expo-auth-session → Google ID-token). Их связывает zustand-стор `useAuth` (restore/login/logout/refresh). Оболочка рендерит `SignInScreen`, пока `status !== 'signedIn'`. Нативная настройка Google (client IDs, URL scheme, config plugin, пересборка dev-build) — документированный ручной шаг, изолированный от тестируемой логики.

**Tech Stack:** Expo SDK 57, React Native 0.86, TypeScript (strict), zustand, jest-expo (jest 29), expo-auth-session + expo-web-browser (Google), expo-secure-store (токены).

**Spec:** `docs/superpowers/specs/2026-08-29-google-auth-design.md` (§B). Серверный контракт: §A.1 (`POST /auth/google {id_token}` → `{access_token, refresh_token, expires_at, user}`; `POST /auth/refresh`; `GET /me`).

## Global Constraints

- Работа в `client/`. Тесты: `npx jest` (preset `jest-expo`), baseline зелёный (16 тестов). Каждая задача заканчивается зелёным `npx jest` и `npx tsc --noEmit`.
- Секретов в коде и в гите нет. Базовый URL сервера — из конфига (`EXPO_PUBLIC_API_URL`, дефолт для локали). `GOOGLE_*_CLIENT_ID` — из конфига (app.json `extra` / env), не в коде.
- Токены (access, refresh) хранятся ТОЛЬКО в `expo-secure-store`, не в AsyncStorage и не в zustand-persist на диск. В памяти стора — можно.
- `expires_at` с сервера — Unix-секунды (int). Клиент парсит как секунды.
- Логика за интерфейсами; нативные вызовы (secure-store, auth-session) — тонкие обёртки, в юнит-тестах подменяются fakes. Google-поток не дёргает сеть в тестах.
- Новые зависимости ставить `npx expo install <pkg>` (совместимые с SDK 57 версии), не `npm install`.
- Стиль: следовать существующему `src/` (zustand в `src/state/`, модели в отдельных файлах, комментарии по-русски как в проекте).

---

## File Structure

- `client/src/net/authClient.ts` — fetch к `/auth/google`, `/auth/refresh`, `/me`; типы ответов. (Create)
- `client/src/net/config.ts` — `apiBaseUrl()` из env/конфига. (Create)
- `client/src/auth/tokenStore.ts` — интерфейс `TokenStore` + реализация на expo-secure-store + `memoryTokenStore` для тестов. (Create)
- `client/src/auth/googleSignIn.ts` — интерфейс `GoogleSignIn` + реализация на expo-auth-session + `fakeGoogleSignIn`. (Create)
- `client/src/state/useAuth.ts` — zustand-стор: `status`, `user`, действия `restore/login/logout/refresh`; зависимости инжектятся. (Create)
- `client/src/screens/auth/SignInScreen.tsx` — экран «Войти через Google». (Create)
- `client/src/screens/auth/ProfileScreen.tsx` — приветствие/выход (опц.). (Create)
- `client/App.tsx` — гейт: `signedIn` → `AppShell`, иначе → `SignInScreen`; `restore()` на старте. (Modify)
- `client/src/**/__tests__/*.test.ts(x)` — тесты. (Create)
- `client/app.json` — `extra` с Google client IDs + scheme; config plugins для auth-session/secure-store. (Modify)
- `client/README.md` — раздел «Auth»: env, нативная настройка Google, пересборка. (Modify)

---

## Task 1: authClient + config

**Files:**
- Create: `client/src/net/config.ts`, `client/src/net/authClient.ts`
- Test: `client/src/net/__tests__/authClient.test.ts`

**Interfaces:**
- Produces:
  - `apiBaseUrl(): string`
  - `type AuthUser = { id: string; email: string; name: string; picture: string }`
  - `type Session = { accessToken: string; refreshToken: string; expiresAt: number; user: AuthUser }`
  - `googleLogin(idToken: string): Promise<Session>`
  - `refresh(refreshToken: string): Promise<Session>`
  - `me(accessToken: string): Promise<AuthUser>`
  - `class AuthError extends Error { status: number }`

- [ ] **Step 1: Write the failing test**

```ts
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd client && npx jest src/net/__tests__/authClient.test.ts`
Expected: FAIL — cannot find module `../authClient`.

- [ ] **Step 3: Write minimal implementation**

`client/src/net/config.ts`:

```ts
// Базовый URL сервера. Из EXPO_PUBLIC_API_URL (доступна в рантайме Expo),
// иначе — локальный дефолт для разработки. Секретов тут нет.
export function apiBaseUrl(): string {
  const fromEnv = process.env.EXPO_PUBLIC_API_URL;
  return (fromEnv && fromEnv.replace(/\/+$/, '')) || 'http://localhost:8080';
}
```

`client/src/net/authClient.ts`:

```ts
import { apiBaseUrl } from './config';

export type AuthUser = { id: string; email: string; name: string; picture: string };
export type Session = {
  accessToken: string;
  refreshToken: string;
  expiresAt: number; // Unix-секунды
  user: AuthUser;
};

export class AuthError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'AuthError';
    this.status = status;
  }
}

type serverSession = {
  access_token: string;
  refresh_token: string;
  expires_at: number;
  user: AuthUser;
};

async function postJSON<T>(path: string, body: unknown, bearer?: string): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (bearer) headers.Authorization = 'Bearer ' + bearer;
  const resp = await fetch(apiBaseUrl() + path, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  });
  if (!resp.ok) {
    throw new AuthError(resp.status, 'auth: ' + path + ' -> ' + resp.status);
  }
  return (await resp.json()) as T;
}

function toSession(s: serverSession): Session {
  return {
    accessToken: s.access_token,
    refreshToken: s.refresh_token,
    expiresAt: s.expires_at,
    user: s.user,
  };
}

export async function googleLogin(idToken: string): Promise<Session> {
  return toSession(await postJSON<serverSession>('/auth/google', { id_token: idToken }));
}

export async function refresh(refreshToken: string): Promise<Session> {
  return toSession(await postJSON<serverSession>('/auth/refresh', { refresh_token: refreshToken }));
}

export async function me(accessToken: string): Promise<AuthUser> {
  const resp = await fetch(apiBaseUrl() + '/me', {
    headers: { Authorization: 'Bearer ' + accessToken },
  });
  if (!resp.ok) throw new AuthError(resp.status, 'auth: /me -> ' + resp.status);
  return (await resp.json()) as AuthUser;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd client && npx jest src/net/__tests__/authClient.test.ts && npx tsc --noEmit`
Expected: PASS; типы чисты.

- [ ] **Step 5: Commit**

```bash
git add client/src/net
git commit -m "feat(client): authClient — /auth/google, /auth/refresh, /me"
```

---

## Task 2: tokenStore (expo-secure-store)

**Files:**
- Create: `client/src/auth/tokenStore.ts`
- Test: `client/src/auth/__tests__/tokenStore.test.ts`

**Interfaces:**
- Consumes: `Session` (Task 1).
- Produces:
  - `type StoredTokens = { accessToken: string; refreshToken: string; expiresAt: number }`
  - `interface TokenStore { load(): Promise<StoredTokens | null>; save(t: StoredTokens): Promise<void>; clear(): Promise<void> }`
  - `function memoryTokenStore(): TokenStore` (для тестов и DEV)
  - `const secureTokenStore: TokenStore` (реализация на expo-secure-store)

- [ ] **Step 1: Install dep**

Run: `cd client && npx expo install expo-secure-store`

- [ ] **Step 2: Write the failing test**

```ts
import { memoryTokenStore } from '../tokenStore';

describe('memoryTokenStore', () => {
  it('save → load round-trips, clear wipes', async () => {
    const s = memoryTokenStore();
    expect(await s.load()).toBeNull();
    await s.save({ accessToken: 'a', refreshToken: 'r', expiresAt: 123 });
    expect(await s.load()).toEqual({ accessToken: 'a', refreshToken: 'r', expiresAt: 123 });
    await s.clear();
    expect(await s.load()).toBeNull();
  });
});
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd client && npx jest src/auth/__tests__/tokenStore.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 4: Write minimal implementation**

`client/src/auth/tokenStore.ts`:

```ts
import * as SecureStore from 'expo-secure-store';

export type StoredTokens = { accessToken: string; refreshToken: string; expiresAt: number };

export interface TokenStore {
  load(): Promise<StoredTokens | null>;
  save(t: StoredTokens): Promise<void>;
  clear(): Promise<void>;
}

const KEY = 'dnd.auth.tokens';

// secureTokenStore — прод-хранилище: токены в защищённом хранилище устройства,
// не в AsyncStorage. Одним ключом, JSON.
export const secureTokenStore: TokenStore = {
  async load() {
    const raw = await SecureStore.getItemAsync(KEY);
    if (!raw) return null;
    try {
      return JSON.parse(raw) as StoredTokens;
    } catch {
      return null;
    }
  },
  async save(t) {
    await SecureStore.setItemAsync(KEY, JSON.stringify(t));
  },
  async clear() {
    await SecureStore.deleteItemAsync(KEY);
  },
};

// memoryTokenStore — для тестов и веб-превью, где secure-store недоступен.
export function memoryTokenStore(): TokenStore {
  let cur: StoredTokens | null = null;
  return {
    async load() {
      return cur;
    },
    async save(t) {
      cur = t;
    },
    async clear() {
      cur = null;
    },
  };
}
```

- [ ] **Step 5: Run test + typecheck**

Run: `cd client && npx jest src/auth/__tests__/tokenStore.test.ts && npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add client/src/auth/tokenStore.ts client/src/auth/__tests__ client/package.json client/package-lock.json
git commit -m "feat(client): tokenStore на expo-secure-store + memory-фейк"
```

---

## Task 3: googleSignIn (expo-auth-session) behind an interface

**Files:**
- Create: `client/src/auth/googleSignIn.ts`
- Test: `client/src/auth/__tests__/googleSignIn.test.ts`

**Interfaces:**
- Produces:
  - `interface GoogleSignIn { signIn(): Promise<string> }` — возвращает Google **ID-token**; бросает при отмене/ошибке.
  - `function fakeGoogleSignIn(idToken: string | Error): GoogleSignIn` (для тестов/DEV)
  - `function useGoogleSignIn(): GoogleSignIn` — реальная реализация на `expo-auth-session/providers/google` (хук; тонкая обёртка).

- [ ] **Step 1: Install deps**

Run: `cd client && npx expo install expo-auth-session expo-web-browser expo-crypto`

- [ ] **Step 2: Write the failing test (fake only — real hook is device-tested)**

```ts
import { fakeGoogleSignIn } from '../googleSignIn';

describe('fakeGoogleSignIn', () => {
  it('returns the id token', async () => {
    const g = fakeGoogleSignIn('id-abc');
    await expect(g.signIn()).resolves.toBe('id-abc');
  });
  it('throws when configured with an error', async () => {
    const g = fakeGoogleSignIn(new Error('cancelled'));
    await expect(g.signIn()).rejects.toThrow('cancelled');
  });
});
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd client && npx jest src/auth/__tests__/googleSignIn.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 4: Write minimal implementation**

`client/src/auth/googleSignIn.ts`:

```ts
import { useMemo } from 'react';
import * as WebBrowser from 'expo-web-browser';
import * as Google from 'expo-auth-session/providers/google';

// GoogleSignIn — граница Google-входа. За ней либо реальный expo-auth-session,
// либо фейк в тестах. Возвращает Google ID-token; бросает при отмене/ошибке.
export interface GoogleSignIn {
  signIn(): Promise<string>;
}

export function fakeGoogleSignIn(result: string | Error): GoogleSignIn {
  return {
    async signIn() {
      if (result instanceof Error) throw result;
      return result;
    },
  };
}

WebBrowser.maybeCompleteAuthSession();

// useGoogleSignIn — реальная обёртка. Client IDs берутся из app.json extra
// (см. README §Auth). ID-token из результата promptAsync. Тонкая: логика
// входа живёт в useAuth, здесь только получение id_token.
export function useGoogleSignIn(): GoogleSignIn {
  const [, , promptAsync] = Google.useIdTokenAuthRequest({
    iosClientId: process.env.EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID,
    androidClientId: process.env.EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID,
    webClientId: process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID,
  });
  return useMemo<GoogleSignIn>(
    () => ({
      async signIn() {
        const res = await promptAsync();
        if (res?.type !== 'success' || !res.params?.id_token) {
          throw new Error('Google вход отменён или без id_token');
        }
        return res.params.id_token as string;
      },
    }),
    [promptAsync],
  );
}
```

- [ ] **Step 5: Run test + typecheck**

Run: `cd client && npx jest src/auth/__tests__/googleSignIn.test.ts && npx tsc --noEmit`
Expected: PASS. (Реальный `useGoogleSignIn` не в тестах — проверяется на устройстве.)

- [ ] **Step 6: Commit**

```bash
git add client/src/auth/googleSignIn.ts client/src/auth/__tests__/googleSignIn.test.ts client/package.json client/package-lock.json
git commit -m "feat(client): googleSignIn за интерфейсом (expo-auth-session) + фейк"
```

---

## Task 4: useAuth (zustand)

**Files:**
- Create: `client/src/state/useAuth.ts`
- Test: `client/src/state/__tests__/useAuth.test.ts`

**Interfaces:**
- Consumes: `authClient` (Task 1), `TokenStore` (Task 2), `GoogleSignIn` (Task 3).
- Produces:
  - `type AuthStatus = 'loading' | 'signedOut' | 'signedIn'`
  - `type AuthDeps = { client: {...}; tokens: TokenStore; google: GoogleSignIn }` — инжект зависимостей (для тестов).
  - `createAuthStore(deps: AuthDeps)` returning a zustand store with `{ status, user, error, restore(), login(), logout(), refresh() }`.
  - `useAuth` — стор по умолчанию с реальными зависимостями.

- [ ] **Step 1: Write the failing test**

```ts
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
    expect(d.google.signIn).toBeUndefined; // sanity: fake used
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

  it('logout: clears tokens → signedOut', async () => {
    const d = deps();
    const store = createAuthStore(d);
    await store.getState().login();
    await store.getState().logout();
    expect(store.getState().status).toBe('signedOut');
    expect(await d.tokens.load()).toBeNull();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd client && npx jest src/state/__tests__/useAuth.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write minimal implementation**

`client/src/state/useAuth.ts`:

```ts
import { createStore } from 'zustand/vanilla';
import { googleLogin, refresh as apiRefresh, me as apiMe, AuthError } from '../net/authClient';
import type { Session, AuthUser } from '../net/authClient';
import { secureTokenStore, TokenStore, StoredTokens } from '../auth/tokenStore';
import { GoogleSignIn } from '../auth/googleSignIn';

export type AuthStatus = 'loading' | 'signedOut' | 'signedIn';

export type AuthClient = {
  googleLogin: (idToken: string) => Promise<Session>;
  refresh: (refreshToken: string) => Promise<Session>;
  me: (accessToken: string) => Promise<AuthUser>;
};

export type AuthDeps = { client: AuthClient; tokens: TokenStore; google: GoogleSignIn };

export type AuthState = {
  status: AuthStatus;
  user: AuthUser | null;
  error: string | null;
  restore: () => Promise<void>;
  login: () => Promise<void>;
  logout: () => Promise<void>;
};

function toStored(s: Session): StoredTokens {
  return { accessToken: s.accessToken, refreshToken: s.refreshToken, expiresAt: s.expiresAt };
}

export function createAuthStore(deps: AuthDeps) {
  return createStore<AuthState>((set) => ({
    status: 'loading',
    user: null,
    error: null,

    async restore() {
      const t = await deps.tokens.load();
      if (!t) {
        set({ status: 'signedOut', user: null });
        return;
      }
      try {
        const user = await deps.client.me(t.accessToken);
        set({ status: 'signedIn', user, error: null });
      } catch (e) {
        if (e instanceof AuthError && e.status === 401) {
          try {
            const s = await deps.client.refresh(t.refreshToken);
            await deps.tokens.save(toStored(s));
            const user = await deps.client.me(s.accessToken);
            set({ status: 'signedIn', user, error: null });
            return;
          } catch {
            await deps.tokens.clear();
          }
        }
        set({ status: 'signedOut', user: null });
      }
    },

    async login() {
      try {
        const idToken = await deps.google.signIn();
        const s = await deps.client.googleLogin(idToken);
        await deps.tokens.save(toStored(s));
        set({ status: 'signedIn', user: s.user, error: null });
      } catch (e) {
        set({ status: 'signedOut', error: (e as Error).message });
      }
    },

    async logout() {
      await deps.tokens.clear();
      set({ status: 'signedOut', user: null, error: null });
    },
  }));
}

// Стор по умолчанию с реальными зависимостями. google подставляется на уровне
// экрана (это хук), поэтому login экрана вызывает store с уже полученным
// GoogleSignIn — см. SignInScreen (Task 5).
export const authStore = createAuthStore({
  client: { googleLogin, refresh: apiRefresh, me: apiMe },
  tokens: secureTokenStore,
  google: { async signIn() { throw new Error('google не подключён'); } },
});
```

> Note: `useGoogleSignIn` is a React hook, so the real GoogleSignIn is obtained in the screen and passed into `login`. Task 5 adjusts `login` to accept an optional `GoogleSignIn` override, or the screen calls a store action `loginWith(google)`. Implement `loginWith(google: GoogleSignIn)` on the store instead of a fixed `google` dep for the default store; keep `login()` for tests that inject `google`. Reconcile: make the store action `login(google?: GoogleSignIn)` using `google ?? deps.google`. Update the test's `login()` calls (they rely on injected `deps.google`) — they still pass.

- [ ] **Step 4: Run test + typecheck**

Run: `cd client && npx jest src/state/__tests__/useAuth.test.ts && npx tsc --noEmit`
Expected: PASS. (Fix the `login(google?)` signature so both the injected-deps tests and the screen path compile.)

- [ ] **Step 5: Commit**

```bash
git add client/src/state/useAuth.ts client/src/state/__tests__/useAuth.test.ts
git commit -m "feat(client): useAuth — restore/login/logout с refresh на 401"
```

---

## Task 5: Гейт оболочки + экраны входа + нативная настройка

**Files:**
- Create: `client/src/screens/auth/SignInScreen.tsx`, `client/src/screens/auth/ProfileScreen.tsx`
- Modify: `client/App.tsx`, `client/app.json`, `client/README.md`
- Test: `client/src/state/__tests__/gate.test.ts`

**Interfaces:**
- Consumes: `authStore`/`createAuthStore`, `useGoogleSignIn`.
- Produces: gating — pure selector `screenFor(status): 'loading' | 'signin' | 'app'`; wired into App.

- [ ] **Step 1: Write the failing test (gating logic, no render)**

```ts
import { screenFor } from '../gate';

describe('screenFor', () => {
  it('maps status to screen', () => {
    expect(screenFor('loading')).toBe('loading');
    expect(screenFor('signedOut')).toBe('signin');
    expect(screenFor('signedIn')).toBe('app');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd client && npx jest src/state/__tests__/gate.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write minimal implementation**

`client/src/state/gate.ts`:

```ts
import type { AuthStatus } from './useAuth';

// screenFor — какой экран показать по статусу авторизации. Чистая функция,
// чтобы гейт был проверяем без рендера.
export function screenFor(status: AuthStatus): 'loading' | 'signin' | 'app' {
  switch (status) {
    case 'signedIn':
      return 'app';
    case 'loading':
      return 'loading';
    default:
      return 'signin';
  }
}
```

`client/src/screens/auth/SignInScreen.tsx` — экран с кнопкой «Войти через Google»: берёт `useGoogleSignIn()`, вызывает `authStore.getState().login(google)`; показывает `error` из стора; стиль из темы (`useTheme`/tokens как в остальных экранах). `ProfileScreen.tsx` — имя/почта из `user`, кнопка «Выйти» → `logout()`.

`client/App.tsx` — на маунте `authStore.getState().restore()`; подписка на `authStore` (через `useStore(authStore)`); `screenFor(status)` → `loading`→сплэш (уже есть ActivityIndicator), `signin`→`<SignInScreen/>`, `app`→`<AppShell/>` (текущий).

- [ ] **Step 4: Install/verify deps + native config (documented manual)**

- `app.json`: добавить `scheme` (для redirect auth-session), `extra` с плейсхолдерами Google client IDs; убедиться, что плагины `expo-secure-store`, `expo-web-browser`/`expo-auth-session` подхвачены (config plugins).
- README §Auth: как завести Google client IDs (iOS/Android/Web) в Google Cloud, куда положить (`EXPO_PUBLIC_GOOGLE_*_CLIENT_ID` + scheme/reversed client id), и что после этого нужен ПЕРЕСБОР dev-build (`npx expo run:ios --device`), т.к. добавились нативные модули (secure-store, auth-session).

- [ ] **Step 5: Run tests + typecheck + full suite**

Run: `cd client && npx jest && npx tsc --noEmit`
Expected: PASS (baseline 16 + new tests). App gating compiles.

- [ ] **Step 6: Commit**

```bash
git add client/src client/App.tsx client/app.json client/README.md
git commit -m "feat(client): гейт входа + SignInScreen/ProfileScreen"
```

---

## Self-Review

**Spec coverage (§B):** Google Sign-In → Task 3; secure-store → Task 2; useAuth (restore/login/logout/refresh) → Task 4; authClient → Task 1; гейт оболочки + SignInScreen(+Profile) → Task 5; токен готов для NetSource (StoredTokens в secure-store) → Tasks 2/4. Нативная настройка Google — Task 5 (документирована, вне логики).

**Placeholder scan:** Task 4 note flags the hook-vs-dep reconciliation (`login(google?)`) with an explicit resolution — implement that signature. No TBD/TODO left as behavior.

**Type consistency:** `Session`/`AuthUser` (Task 1) reused by Tasks 4/5; `TokenStore`/`StoredTokens` (Task 2) by Task 4; `GoogleSignIn` (Task 3) by Tasks 4/5; `AuthStatus` (Task 4) by Task 5 `screenFor`. Server field mapping (`access_token`→`accessToken`, `expires_at` unix-seconds) centralised in `toSession`.

## Границы (не здесь)

- `NetSource` (живой сокет клиента ↔ сервер, замена MockSource) — отдельный эпик, следующий по плану.
- Реальный e2e Google-поток на устройстве (нужны client IDs + пересборка) — ручная проверка по README, не юнит-тест.
- Email/пароль, мультиаккаунт, deep-link на кастомные схемы сверх auth redirect.
