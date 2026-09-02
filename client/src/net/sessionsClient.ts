import { apiBaseUrl } from './config';

export type SessionSummary = { chatId: string; caseId: string; seed: number; createdAt: number };

export class SessionsError extends Error {
  status: number;
  constructor(status: number, message: string) { super(message); this.name = 'SessionsError'; this.status = status; }
}

async function req<T>(path: string, init: RequestInit, token: string): Promise<T> {
  const resp = await fetch(apiBaseUrl() + path, {
    ...init,
    headers: { ...(init.headers ?? {}), Authorization: 'Bearer ' + token },
  });
  if (!resp.ok) throw new SessionsError(resp.status, 'sessions: ' + path + ' -> ' + resp.status);
  return (await resp.json()) as T;
}

type serverSummary = { chat_id: string; case_id: string; seed: number; created_at: number };

export async function listSessions(token: string): Promise<SessionSummary[]> {
  const raw = await req<serverSummary[]>('/sessions', { method: 'GET' }, token);
  return (raw ?? []).map((s) => ({ chatId: s.chat_id, caseId: s.case_id, seed: s.seed, createdAt: s.created_at }));
}

// createSession — POST /sessions. character_id обязателен на сервере (см.
// server/server.go: createSessionRequest) — без него неоткуда взять лист
// персонажа и систему правил дела.
export async function createSession(token: string, caseId: string, characterId: string, seed?: number): Promise<{ chatId: string }> {
  const body: Record<string, unknown> = { case_id: caseId, character_id: characterId };
  if (seed !== undefined) body.seed = seed;
  const raw = await req<{ chat_id: string }>('/sessions', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  }, token);
  return { chatId: raw.chat_id };
}
