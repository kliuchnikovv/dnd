import { createStore } from 'zustand/vanilla';
import { useStore } from 'zustand';
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
  // google не обязателен: тесты инжектируют deps.google и зовут login() без
  // аргумента; экран (Task 5) получает GoogleSignIn из React-хука и передаёт
  // его явно — login(hookProvider).
  login: (google?: GoogleSignIn) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
};

function toStored(s: Session): StoredTokens {
  return { accessToken: s.accessToken, refreshToken: s.refreshToken, expiresAt: s.expiresAt };
}

export function createAuthStore(deps: AuthDeps) {
  return createStore<AuthState>((set, get) => ({
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
        const status = e instanceof AuthError ? e.status : (e as { status?: number })?.status;
        if (status === 401) {
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
        // Не-401 ошибка (например, сетевой сбой) не трогает токены — можно
        // повторить restore() позже без повторного логина.
        set({ status: 'signedOut', user: null });
      }
    },

    async login(google?: GoogleSignIn) {
      const g = google ?? deps.google;
      try {
        const idToken = await g.signIn();
        const s = await deps.client.googleLogin(idToken);
        await deps.tokens.save(toStored(s));
        set({ status: 'signedIn', user: s.user, error: null });
      } catch (e) {
        await deps.tokens.clear();
        set({ status: 'signedOut', user: null, error: (e as Error).message });
      }
    },

    async logout() {
      await deps.tokens.clear();
      set({ status: 'signedOut', user: null, error: null });
    },

    async refresh() {
      const t = await deps.tokens.load();
      if (!t) {
        set({ status: 'signedOut', user: null });
        return;
      }
      try {
        const s = await deps.client.refresh(t.refreshToken);
        await deps.tokens.save(toStored(s));
        set({ status: 'signedIn', error: null });
      } catch (e) {
        await deps.tokens.clear();
        set({ status: 'signedOut', user: null, error: (e as Error).message });
      }
    },
  }));
}

// Стор по умолчанию с реальными зависимостями. Реальный GoogleSignIn — React-
// хук (useGoogleSignIn), поэтому здесь заглушка: экран (Task 5) получает его
// из хука и передаёт явно в login(hookProvider).
export const authStore = createAuthStore({
  client: { googleLogin, refresh: apiRefresh, me: apiMe },
  tokens: secureTokenStore,
  google: { async signIn() { throw new Error('google не подключён — передайте GoogleSignIn в login()'); } },
});

export function useAuth(): AuthState {
  return useStore(authStore);
}
