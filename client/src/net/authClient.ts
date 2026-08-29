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
