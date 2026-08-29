# NetSource — живое подключение клиента к серверу. План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Клиент играет через реальный сервер: выбор/создание сессии, ход через WebSocket, стрим прозы — за тем же `TurnViewSource`, что и моки.

**Architecture:** Три слайса. (A) сервер отдаёт список сессий владельца (`GET /sessions`). (B) `NetSource` реализует `TurnViewSource` поверх WS Frame-протокола (connect → `session_state`; `send` резолвится следующим `session_state`; `data`-дельты копятся в прозу; реконнект/auth-рефреш), плюс REST-клиент сессий. (C) экран выбора сессии и контейнер, владеющий жизненным циклом NetSource; turn-view-экраны не меняются.

**Tech Stack:** Go 1.26 (сервер), TypeScript/React Native/Expo SDK 57, zustand, jest-expo, WebSocket, pgx.

**Spec:** `docs/superpowers/specs/2026-08-29-netsource-design.md`

## Global Constraints

- `NetSource` реализует `TurnViewSource` (`current(): TurnView`, `send(intent): Promise<TurnView>`, `subscribe(l): () => void`) — экраны turn-view не меняются.
- `Intent` = `{ kind: 'token'; token: string } | { kind: 'free'; text: string }` (client/src/turnview/intents.ts). Маппинг в кадр: token→`{token}`, free→`{text}`.
- `Frame` побайтово совместим с сервером: `{ id:number; chat_id:string; channel:string; kind:string; op:string; payload?:any; error?:any }` (server/frame.go).
- Клиент не решает исход: `send` только шлёт; состояние приходит `session_state`'ом от сервера.
- Всё под Bearer-токеном из `useAuth`; WS — токен в query `?token=&chat_id=`.
- Секретов в коде нет; базовый URL из `apiBaseUrl()` / `EXPO_PUBLIC_API_URL`; WS-URL выводится из него (`http→ws`, `https→wss`).
- Клиент: `cd client && npx jest` зелёный + `npx tsc --noEmit` чистый. Сервер: `go test ./...` зелёный; pg-тесты gated по `DATABASE_URL`.

---

## File Structure

- `server/store.go` (M) — `SessionRecord.CreatedAt`; `Store.SessionsByUser`; memStore impl.
- `server/pgstore.go` (M) — pg `SessionsByUser`; `created_at` в SELECT/scan `Sessions`.
- `server/authhttp.go` или `server/server.go` (M) — хендлер `GET /sessions` + регистрация.
- `client/src/net/frame.ts` (C) — `Frame` тип + константы.
- `client/src/net/sessionsClient.ts` (C) — `listSessions`, `createSession`.
- `client/src/net/netSource.ts` (C) — `NetSource`, `WebSocketLike`.
- `client/src/screens/session/SessionSelectScreen.tsx` (C) — экран выбора + чистая логика вьюмодели.
- `client/src/screens/session/SessionScreen.tsx` (C) — контейнер жизненного цикла.
- `client/App.tsx` (M) — гейт signedIn → выбор → игра; `client/src/state/store.ts` (M) — `source: TurnViewSource`.

---

## Task 1: сервер — `GET /sessions` (список сессий владельца)

**Files:**
- Modify: `server/store.go`, `server/pgstore.go`, `server/server.go` (+ `server/authhttp.go` для хендлера)
- Test: `server/authgate_test.go` (добавить), `server/pgstore_test.go` (добавить gated)

**Interfaces:**
- Consumes: `SessionRecord`, `Store`, `requireAuth`, `userIDFrom` (уже есть).
- Produces: `SessionRecord.CreatedAt time.Time`; `Store.SessionsByUser(ctx, userID string) ([]SessionRecord, error)`; `GET /sessions` → `200 [{chat_id,case_id,seed,created_at}]`.

- [ ] **Step 1: Write the failing test (mem + handler)**

Add to `server/authgate_test.go`:

```go
func TestListSessionsReturnsOnlyOwn(t *testing.T) {
	srv, svc := testServerWithAuth(t) // helper from Task 6 of auth (authedServer)
	a, _ := svc.LoginAs(context.Background(), auth.Identity{Sub: "gA", Email: "a@x"})
	b, _ := svc.LoginAs(context.Background(), auth.Identity{Sub: "gB", Email: "b@x"})
	idA := createSessionAs(t, srv, a.AccessToken, "harbour") // helper posts /sessions with Bearer
	createSessionAs(t, srv, b.AccessToken, "harbour")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+a.AccessToken)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("код %d тело %q", rec.Code, rec.Body.String())
	}
	var out []struct {
		ChatID string `json:"chat_id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || out[0].ChatID != idA {
		t.Fatalf("ждали только сессию A (%s), получили %+v", idA, out)
	}
}

func TestListSessionsRequiresAuth(t *testing.T) {
	srv, _ := testServerWithAuth(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/sessions", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("без токена ждали 401, получили %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./server/ -run 'TestListSessions' -v`
Expected: FAIL — no `GET /sessions` route / `SessionsByUser` undefined.

- [ ] **Step 3: Implement**

`server/store.go`: add `CreatedAt time.Time` to `SessionRecord`; add to interface + memStore:

```go
// В Store interface:
SessionsByUser(ctx context.Context, userID string) ([]SessionRecord, error)

// memStore:
func (s *memStore) SessionsByUser(_ context.Context, userID string) ([]SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []SessionRecord
	for _, r := range s.sessions {
		if userID != "" && r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}
```

`server/pgstore.go`: implement pg `SessionsByUser` and add `created_at` to the existing `Sessions` SELECT/scan:

```go
func (s *pgStore) SessionsByUser(ctx context.Context, userID string) ([]SessionRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT chat_id, case_id, seed, snapshot, core_version, user_id, created_at
		FROM sessions WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionRecord
	for rows.Next() {
		var r SessionRecord
		var uid *string
		if err := rows.Scan(&r.ChatID, &r.CaseID, &r.Seed, &r.Snapshot, &r.CoreVersion, &uid, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.UserID = deref(uid)
		out = append(out, r)
	}
	return out, rows.Err()
}
```

(Also add `user_id, created_at` to the existing `Sessions` SELECT + scan so `CreatedAt`/`UserID` fill there too; `Sessions` orders don't matter.)

`server/authhttp.go`: handler + register in `New` (inside `if s.auth != nil`) as `s.mux.HandleFunc("GET /sessions", s.requireAuth(s.handleListSessions))`:

```go
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	recs, err := s.mgr.store.SessionsByUser(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать сессии")
		return
	}
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		out = append(out, map[string]any{
			"chat_id": r.ChatID, "case_id": r.CaseID,
			"seed": r.Seed, "created_at": r.CreatedAt.Unix(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}
```

> Note: `s.mgr.store` is unexported access within package `server` — fine (handler is in package server). If `Manager.store` field name differs, use it. Existing `POST /sessions` stays for "new game".

- [ ] **Step 4: gated pg test**

Add to `server/pgstore_test.go`:

```go
func TestPgStoreSessionsByUser(t *testing.T) {
	st := pgStoreForTest(t)
	defer st.Close(context.Background())
	ctx := context.Background()
	uid := "u-" + randToken()
	_ = st.UpsertUser(ctx, UserRecord{ID: uid, Email: "o@x"})
	_ = st.SaveSession(ctx, SessionRecord{ChatID: "c-" + randToken(), CaseID: "harbour", Seed: 1, UserID: uid, CoreVersion: "core-1", Snapshot: "s"})
	got, err := st.SessionsByUser(ctx, uid)
	if err != nil || len(got) != 1 {
		t.Fatalf("SessionsByUser: %d rows err=%v", len(got), err)
	}
	if got[0].UserID != uid || got[0].CreatedAt.IsZero() {
		t.Fatalf("owner/created_at не заполнены: %+v", got[0])
	}
}
```

- [ ] **Step 5: Verify + commit**

Run: `go test ./server/` (green; pg tests skip). Controller runs pg live.
```bash
git add server/store.go server/pgstore.go server/authhttp.go server/server.go server/authgate_test.go server/pgstore_test.go
git commit -m "feat(server): GET /sessions — список сессий владельца"
```

---

## Task 2: клиент — sessionsClient (REST)

**Files:**
- Create: `client/src/net/sessionsClient.ts`, `client/src/net/__tests__/sessionsClient.test.ts`

**Interfaces:**
- Consumes: `apiBaseUrl()` (client/src/net/config.ts).
- Produces: `SessionSummary { chatId:string; caseId:string; seed:number; createdAt:number }`; `listSessions(token:string): Promise<SessionSummary[]>`; `createSession(token:string, caseName:string, seed?:number): Promise<{ chatId:string }>`.

- [ ] **Step 1: Write the failing test**

```ts
import { listSessions, createSession } from '../sessionsClient';

function mockFetch(status: number, body: unknown, capture?: (url: string, opts: any) => void) {
  (global as any).fetch = jest.fn(async (url: string, opts: any) => {
    capture?.(url, opts);
    return { ok: status >= 200 && status < 300, status, json: async () => body };
  });
}

test('listSessions maps snake_case and sends Bearer', async () => {
  let seenAuth = '';
  mockFetch(200, [{ chat_id: 'c1', case_id: 'harbour', seed: 1, created_at: 1000 }],
    (_u, o) => { seenAuth = o.headers.Authorization; });
  const out = await listSessions('tok');
  expect(seenAuth).toBe('Bearer tok');
  expect(out).toEqual([{ chatId: 'c1', caseId: 'harbour', seed: 1, createdAt: 1000 }]);
});

test('createSession posts case and returns chatId', async () => {
  let body: any;
  mockFetch(200, { chat_id: 'c2' }, (_u, o) => { body = JSON.parse(o.body); });
  const out = await createSession('tok', 'harbour');
  expect(body.case).toBe('harbour');
  expect(out).toEqual({ chatId: 'c2' });
});

test('non-2xx throws with status', async () => {
  mockFetch(401, { error: 'no' });
  await expect(listSessions('tok')).rejects.toMatchObject({ status: 401 });
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd client && npx jest src/net/__tests__/sessionsClient.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

```ts
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

export async function createSession(token: string, caseName: string, seed?: number): Promise<{ chatId: string }> {
  const body: Record<string, unknown> = { case: caseName };
  if (seed !== undefined) body.seed = seed;
  const raw = await req<{ chat_id: string }>('/sessions', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  }, token);
  return { chatId: raw.chat_id };
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd client && npx jest src/net/__tests__/sessionsClient.test.ts && npx jest && npx tsc --noEmit`
Expected: PASS; suite green; tsc clean.

- [ ] **Step 5: Commit**

```bash
git add client/src/net/sessionsClient.ts client/src/net/__tests__/sessionsClient.test.ts
git commit -m "feat(client): sessionsClient — список и создание сессий"
```

---

## Task 3: клиент — Frame + NetSource ядро (connect, session_state, current, subscribe)

**Files:**
- Create: `client/src/net/frame.ts`, `client/src/net/netSource.ts`, `client/src/net/__tests__/netSource.test.ts`

**Interfaces:**
- Consumes: `TurnView` (client/src/turnview/types.ts), `TurnViewSource`/`Intent` (turnview), `Frame` (this task).
- Produces:
  - `Frame` type + `WebSocketLike` interface + `NetSourceDeps { chatId:string; wsUrl:string; getToken(): Promise<string>; onAuthLost(): void; makeSocket?: (url:string) => WebSocketLike }`.
  - `class NetSource implements TurnViewSource` with `connect(): void`, `close(): void` beyond the interface.
  - Placeholder view: `{ version: 0, scene: { node: '', title: '' }, options: [] }`.

- [ ] **Step 1: Write the failing test**

```ts
import { NetSource, WebSocketLike } from '../netSource';
import { TurnView } from '../../turnview/types';

class FakeSocket implements WebSocketLike {
  onopen: (() => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  sent: string[] = [];
  send(d: string) { this.sent.push(d); }
  close() { this.onclose?.(); }
  push(frame: unknown) { this.onmessage?.({ data: JSON.stringify(frame) }); }
  open() { this.onopen?.(); }
}

function sessionStateFrame(tv: Partial<TurnView>) {
  return { id: 1, chat_id: 'c', channel: 'chat', kind: 'data', op: 'session_state',
    payload: { version: 1, scene: { node: 'n_quay', title: 'Причал' }, options: [{ id: 'o1', label: 'Осмотреть', token: 'tok1' }], ...tv } };
}

function makeNet() {
  const fake = new FakeSocket();
  const net = new NetSource({
    chatId: 'c', wsUrl: 'ws://x/chat/ws',
    getToken: async () => 'tok', onAuthLost: () => {},
    makeSocket: () => fake,
  });
  return { net, fake };
}

test('current() is a placeholder until session_state arrives', () => {
  const { net } = makeNet();
  expect(net.current().version).toBe(0);
});

test('session_state updates current() and notifies subscribers', () => {
  const { net, fake } = makeNet();
  const seen: TurnView[] = [];
  net.subscribe((v) => seen.push(v));
  net.connect();
  fake.open();
  fake.push(sessionStateFrame({}));
  expect(net.current().scene.title).toBe('Причал');
  expect(seen[seen.length - 1].options?.[0].token).toBe('tok1');
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd client && npx jest src/net/__tests__/netSource.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement frame.ts + NetSource core**

`client/src/net/frame.ts`:
```ts
export interface Frame {
  id: number;
  chat_id: string;
  channel: string;
  kind: string;
  op: string;
  payload?: any;
  error?: any;
}
export const Channel = { chat: 'chat', user: 'user' } as const;
export const Kind = { data: 'data', meta: 'meta', signal: 'signal', error: 'error' } as const;
export const Op = {
  message: 'message', ping: 'ping', done: 'done',
  history: 'history', sessionState: 'session_state', stop: 'stop',
} as const;
```

`client/src/net/netSource.ts` (core — connect/session_state/current/subscribe; send & reconnect & auth added in Tasks 4–5):
```ts
import { TurnView } from '../turnview/types';
import { Intent } from '../turnview/intents';
import { TurnViewSource } from '../turnview/source';
import { Frame, Kind, Op } from './frame';

export interface WebSocketLike {
  onopen: (() => void) | null;
  onmessage: ((e: { data: string }) => void) | null;
  onclose: (() => void) | null;
  onerror: (() => void) | null;
  send(data: string): void;
  close(): void;
}

export interface NetSourceDeps {
  chatId: string;
  wsUrl: string;
  getToken(): Promise<string>;
  onAuthLost(): void;
  makeSocket?: (url: string) => WebSocketLike;
}

const PLACEHOLDER: TurnView = { version: 0, scene: { node: '', title: '' }, options: [] };

export class NetSource implements TurnViewSource {
  private view: TurnView = PLACEHOLDER;
  private listeners = new Set<(v: TurnView) => void>();
  private ws: WebSocketLike | null = null;
  private prose = ''; // накопитель прозы текущего хода

  constructor(private deps: NetSourceDeps) {}

  current(): TurnView { return this.view; }

  subscribe(listener: (v: TurnView) => void): () => void {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  }

  async connect(): Promise<void> {
    const token = await this.deps.getToken();
    const url = `${this.deps.wsUrl}?token=${encodeURIComponent(token)}&chat_id=${encodeURIComponent(this.deps.chatId)}`;
    const make = this.deps.makeSocket ?? ((u: string) => new WebSocket(u) as unknown as WebSocketLike);
    const ws = make(url);
    this.ws = ws;
    ws.onmessage = (e) => this.onFrame(JSON.parse(e.data) as Frame);
    ws.onopen = () => {};
  }

  close(): void { this.ws?.close(); this.ws = null; }

  // send() — Task 4. Реализуй здесь по Task 4.
  send(_intent: Intent): Promise<TurnView> { return Promise.resolve(this.view); }

  private onFrame(f: Frame): void {
    if (f.op === Op.sessionState && f.kind === Kind.data) {
      this.prose = '';
      this.view = f.payload as TurnView;
      this.emit();
    }
    // message/data, done, history, ping, error — Tasks 4–5.
  }

  private emit(): void { this.listeners.forEach((l) => l(this.view)); }
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd client && npx jest src/net/__tests__/netSource.test.ts && npx tsc --noEmit`
Expected: PASS; tsc clean.

- [ ] **Step 5: Commit**

```bash
git add client/src/net/frame.ts client/src/net/netSource.ts client/src/net/__tests__/netSource.test.ts
git commit -m "feat(client): Frame + NetSource ядро — connect и session_state"
```

---

## Task 4: NetSource — send-корреляция, стрим прозы, done, history

**Files:**
- Modify: `client/src/net/netSource.ts`, `client/src/net/__tests__/netSource.test.ts`

**Interfaces:**
- Consumes: NetSource core (Task 3), `Intent`, `Frame`.
- Produces: working `send(intent)` (resolves on next `session_state`); prose accumulation into `view.narration`; `done`; `history`.

- [ ] **Step 1: Write the failing tests**

```ts
test('send maps token intent to a message frame and resolves on next session_state', async () => {
  const { net, fake } = makeNet();
  net.connect(); fake.open();
  fake.push(sessionStateFrame({}));
  const p = net.send({ kind: 'token', token: 'tok1' });
  const sent = JSON.parse(fake.sent[fake.sent.length - 1]);
  expect(sent.op).toBe('message');
  expect(sent.payload.token).toBe('tok1');
  fake.push(sessionStateFrame({ scene: { node: 'n_forge', title: 'Кузница' } }));
  const view = await p;
  expect(view.scene.title).toBe('Кузница');
});

test('free intent maps to text payload', async () => {
  const { net, fake } = makeNet();
  net.connect(); fake.open(); fake.push(sessionStateFrame({}));
  const p = net.send({ kind: 'free', text: '1' });
  const sent = JSON.parse(fake.sent[fake.sent.length - 1]);
  expect(sent.payload.text).toBe('1');
  fake.push(sessionStateFrame({}));
  await p;
});

test('prose deltas accumulate into narration; done finalizes', () => {
  const { net, fake } = makeNet();
  net.connect(); fake.open(); fake.push(sessionStateFrame({}));
  fake.push({ id: 2, chat_id: 'c', channel: 'chat', kind: 'data', op: 'message', payload: { type: 'text', delta: 'Прич', ix: 0 } });
  fake.push({ id: 3, chat_id: 'c', channel: 'chat', kind: 'data', op: 'message', payload: { type: 'text', delta: 'ал в тумане', ix: 1 } });
  expect(net.current().narration?.[0].text).toBe('Причал в тумане');
  expect(net.current().narration?.[0].streaming).toBe(true);
  fake.push({ id: 4, chat_id: 'c', channel: 'chat', kind: 'meta', op: 'done' });
  expect(net.current().narration?.[0].streaming).toBe(false);
});

test('history frame seeds finished narration on reconnect', () => {
  const { net, fake } = makeNet();
  net.connect(); fake.open(); fake.push(sessionStateFrame({}));
  fake.push({ id: 5, chat_id: 'c', channel: 'chat', kind: 'data', op: 'history', payload: { type: 'narration', text: 'Готовая проза' } });
  expect(net.current().narration?.[0].text).toBe('Готовая проза');
  expect(net.current().narration?.[0].streaming).toBe(false);
});
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd client && npx jest src/net/__tests__/netSource.test.ts`
Expected: FAIL — send resolves current view immediately / no prose handling.

- [ ] **Step 3: Implement**

In `netSource.ts`: add an outgoing id counter, a pending resolver, and extend `onFrame`:

```ts
// fields:
private outId = 0;
private pending: ((v: TurnView) => void) | null = null;

send(intent: Intent): Promise<TurnView> {
  if (!this.ws) return Promise.reject(new Error('netSource: не подключено'));
  const payload = intent.kind === 'token' ? { token: intent.token } : { text: intent.text };
  this.outId += 1;
  const frame: Frame = { id: this.outId, chat_id: this.deps.chatId, channel: 'chat', kind: Kind.data, op: Op.message, payload };
  this.ws.send(JSON.stringify(frame));
  return new Promise<TurnView>((resolve) => { this.pending = resolve; });
}
```

Extend `onFrame`:
```ts
private onFrame(f: Frame): void {
  switch (f.op) {
    case Op.sessionState:
      this.prose = '';
      this.view = f.payload as TurnView;
      this.emit();
      if (this.pending) { const r = this.pending; this.pending = null; r(this.view); }
      break;
    case Op.message: // проза-дельта
      if (f.kind === Kind.data && f.payload?.type === 'text') {
        this.prose += f.payload.delta ?? '';
        this.view = { ...this.view, narration: [{ kind: 'gm', text: this.prose, streaming: true }] };
        this.emit();
      }
      break;
    case Op.done:
      this.view = { ...this.view, narration: this.prose ? [{ kind: 'gm', text: this.prose, streaming: false }] : this.view.narration };
      this.emit();
      break;
    case Op.history:
      if (f.payload?.type === 'narration') {
        this.prose = f.payload.text ?? '';
        this.view = { ...this.view, narration: [{ kind: 'gm', text: this.prose, streaming: false }] };
        this.emit();
      }
      break;
    case Op.ping:
      this.ws?.send(JSON.stringify({ id: 0, chat_id: this.deps.chatId, channel: 'chat', kind: Kind.signal, op: Op.ping }));
      break;
  }
}
```

- [ ] **Step 4: Run to verify they pass**

Run: `cd client && npx jest src/net/__tests__/netSource.test.ts && npx tsc --noEmit`
Expected: PASS; tsc clean.

- [ ] **Step 5: Commit**

```bash
git add client/src/net/netSource.ts client/src/net/__tests__/netSource.test.ts
git commit -m "feat(client): NetSource — ход, стрим прозы, done, history"
```

---

## Task 5: NetSource — реконнект + auth-рефреш

**Files:**
- Modify: `client/src/net/netSource.ts`, `client/src/net/__tests__/netSource.test.ts`

**Interfaces:**
- Consumes: NetSource (Tasks 3–4).
- Produces: авто-реконнект после неожиданного `close`; на отказе авторизации — один рефреш через `getToken`, затем реконнект; повторный отказ → `onAuthLost()`. `close()` глушит реконнект.

- [ ] **Step 1: Write the failing tests**

```ts
test('reconnects after unexpected close and re-applies session_state', async () => {
  let sockets: FakeSocket[] = [];
  const net = new NetSource({
    chatId: 'c', wsUrl: 'ws://x/chat/ws', getToken: async () => 'tok', onAuthLost: () => {},
    makeSocket: () => { const f = new FakeSocket(); sockets.push(f); return f; },
  });
  // без реальных таймеров: инжектируй немедленный планировщик (см. реализацию: deps.schedule)
  (net as any).schedule = (fn: () => void) => fn();
  await net.connect(); sockets[0].open(); sockets[0].push(sessionStateFrame({}));
  sockets[0].onclose?.(); // неожиданный обрыв
  await Promise.resolve();
  expect(sockets.length).toBe(2); // переподключился
  sockets[1].open(); sockets[1].push(sessionStateFrame({ scene: { node: 'n2', title: 'После' } }));
  expect(net.current().scene.title).toBe('После');
});

test('close() stops reconnect', async () => {
  let sockets: FakeSocket[] = [];
  const net = new NetSource({ chatId: 'c', wsUrl: 'ws://x', getToken: async () => 'tok', onAuthLost: () => {},
    makeSocket: () => { const f = new FakeSocket(); sockets.push(f); return f; } });
  (net as any).schedule = (fn: () => void) => fn();
  await net.connect(); sockets[0].open();
  net.close();
  sockets[0].onclose?.();
  await Promise.resolve();
  expect(sockets.length).toBe(1); // нет реконнекта после close()
});
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd client && npx jest src/net/__tests__/netSource.test.ts`
Expected: FAIL — no reconnect.

- [ ] **Step 3: Implement**

Add a `schedule` seam (default `setTimeout` with backoff; tests override), a `closed` flag, and reconnect on `onclose`:

```ts
// fields:
private closed = false;
private backoff = 1000;
private schedule: (fn: () => void, ms: number) => void = (fn, ms) => { setTimeout(fn, ms); };

// в connect(), после создания ws:
ws.onclose = () => {
  this.ws = null;
  if (this.closed) return;
  const wait = this.backoff;
  this.backoff = Math.min(this.backoff * 2, 30000);
  this.schedule(() => { void this.connect(); }, wait);
};
ws.onopen = () => { this.backoff = 1000; };

// close():
close(): void { this.closed = true; this.ws?.close(); this.ws = null; }
```

Auth-refresh: if the socket errors/handshake rejects with auth (`onerror` or an `error` frame with 401), call `getToken()` again (it triggers the app's refresh) and reconnect; if the next connect also auth-fails, call `deps.onAuthLost()`. Minimal: track `authRetried` — on an `error` frame `code===401` or an `onerror` immediately after connect, if `!authRetried` set it and reconnect; else `onAuthLost()`. Add a test driving an `error` 401 frame → `getToken` called again; second 401 → `onAuthLost` called. (Model with the fake socket + a `getToken` jest.fn and an `onAuthLost` jest.fn.)

- [ ] **Step 4: Run to verify they pass**

Run: `cd client && npx jest src/net/__tests__/netSource.test.ts && npx jest && npx tsc --noEmit`
Expected: PASS; whole suite green; tsc clean.

- [ ] **Step 5: Commit**

```bash
git add client/src/net/netSource.ts client/src/net/__tests__/netSource.test.ts
git commit -m "feat(client): NetSource — реконнект и auth-рефреш"
```

---

## Task 6: SessionSelectScreen + чистая логика выбора

**Files:**
- Create: `client/src/screens/session/sessionSelect.ts` (pure vm), `client/src/screens/session/SessionSelectScreen.tsx`, `client/src/screens/session/__tests__/sessionSelect.test.ts`

**Interfaces:**
- Consumes: `SessionSummary`, `listSessions`, `createSession` (Task 2).
- Produces: pure `toSessionRows(list: SessionSummary[], now: number): SessionRow[]` (`SessionRow { chatId, caseId, subtitle }` — subtitle = relative time) for unit testing; `SessionSelectScreen` component.

- [ ] **Step 1: Write the failing test (pure logic)**

```ts
import { toSessionRows } from '../sessionSelect';

test('maps sessions to rows with relative time, newest first assumed pre-sorted', () => {
  const now = 10_000; // seconds
  const rows = toSessionRows(
    [{ chatId: 'c1', caseId: 'harbour', seed: 1, createdAt: 10_000 - 3600 }], now);
  expect(rows[0].chatId).toBe('c1');
  expect(rows[0].caseId).toBe('harbour');
  expect(rows[0].subtitle).toMatch(/час|hour|1/); // relative-time string, non-empty
  expect(rows[0].subtitle.length).toBeGreaterThan(0);
});

test('empty list → empty rows', () => {
  expect(toSessionRows([], 10_000)).toEqual([]);
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd client && npx jest src/screens/session/__tests__/sessionSelect.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement pure logic + screen**

`sessionSelect.ts`:
```ts
import { SessionSummary } from '../../net/sessionsClient';

export type SessionRow = { chatId: string; caseId: string; subtitle: string };

export function relativeTime(fromUnix: number, nowUnix: number): string {
  const s = Math.max(0, nowUnix - fromUnix);
  if (s < 60) return 'только что';
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} мин назад`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} ч назад`;
  return `${Math.floor(h / 24)} дн назад`;
}

export function toSessionRows(list: SessionSummary[], nowUnix: number): SessionRow[] {
  return list.map((s) => ({ chatId: s.chatId, caseId: s.caseId, subtitle: relativeTime(s.createdAt, nowUnix) }));
}
```

`SessionSelectScreen.tsx`: on mount `listSessions(token)` → `toSessionRows` → render list (theme + ds ListItem) + «Новая игра» button (`createSession(token, 'harbour')`); each row/новая → `onPick(chatId)` prop. Spinner while loading, error line on failure. Token via `useAuth`. (Render is device-tested; the pure vm is unit-tested.)

- [ ] **Step 4: Run to verify it passes**

Run: `cd client && npx jest src/screens/session/__tests__/sessionSelect.test.ts && npx jest && npx tsc --noEmit`
Expected: PASS; suite green; tsc clean.

- [ ] **Step 5: Commit**

```bash
git add client/src/screens/session/
git commit -m "feat(client): SessionSelectScreen — список и новая игра"
```

---

## Task 7: SessionScreen контейнер + гейт приложения + store

**Files:**
- Create: `client/src/screens/session/SessionScreen.tsx`
- Modify: `client/App.tsx`, `client/src/state/store.ts`
- Test: `client/src/state/__tests__/store.test.ts` (тип-совместимость), либо покрытие через существующие тесты

**Interfaces:**
- Consumes: `NetSource` (Tasks 3–5), `SessionSelectScreen` (Task 6), `useAuth` (client/src/state/useAuth.ts), `AppShell`, `useTurnView`.
- Produces: `SessionScreen` (контейнер), обновлённый App-гейт `signedIn → выбор → игра`, `store.source: TurnViewSource`.

- [ ] **Step 1: Write the failing test (store retype + wsUrl helper)**

Add a small pure helper `wsUrlFrom(apiBase)` and test it (deterministic, no RN):

```ts
import { wsUrlFrom } from '../../net/netSource';

test('wsUrlFrom converts http→ws and https→wss and appends /chat/ws', () => {
  expect(wsUrlFrom('http://localhost:8080')).toBe('ws://localhost:8080/chat/ws');
  expect(wsUrlFrom('https://api.example.com')).toBe('wss://api.example.com/chat/ws');
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd client && npx jest -t wsUrlFrom`
Expected: FAIL — `wsUrlFrom` undefined.

- [ ] **Step 3: Implement**

Add `wsUrlFrom` to `netSource.ts`:
```ts
export function wsUrlFrom(apiBase: string): string {
  const base = apiBase.replace(/\/+$/, '');
  const ws = base.startsWith('https://') ? base.replace('https://', 'wss://')
           : base.startsWith('http://') ? base.replace('http://', 'ws://') : base;
  return ws + '/chat/ws';
}
```

`SessionScreen.tsx`: props `{ chatId: string }`. On mount build `const net = new NetSource({ chatId, wsUrl: wsUrlFrom(apiBaseUrl()), getToken: async () => useAuth.getState().accessToken(), onAuthLost: () => useAuth.getState().logout() })` (adapt to the actual useAuth token accessor), `net.connect()`; on unmount `net.close()`. Render: spinner while `view.version === 0` (subscribe via `useTurnView(net)`), reconnect banner on error, else the existing `AppShell`/turn-view. Keep `net` stable across renders (`useRef`/`useMemo`).

`store.ts`: change `source: MockSource` → `source: TurnViewSource` (import the interface); keep `createHavenSource()` as a dev fallback (e.g. behind a flag), production path uses the NetSource from SessionScreen.

`App.tsx`: extend the existing gate — `signedIn` → if no `chatId` chosen, `<SessionSelectScreen onPick={setChatId} />`; once chosen, `<SessionScreen chatId={chatId} />`. Keep the fonts-gate and SignInScreen branch. Hold `chatId` in a small state (local or a tiny zustand slice).

- [ ] **Step 4: Run to verify it passes**

Run: `cd client && npx jest && npx tsc --noEmit`
Expected: PASS; whole suite green; tsc clean.

- [ ] **Step 5: Commit**

```bash
git add client/src/screens/session/SessionScreen.tsx client/App.tsx client/src/state/store.ts client/src/net/netSource.ts client/src/net/__tests__/
git commit -m "feat(client): SessionScreen контейнер + гейт выбор→игра"
```

---

## Self-Review

**Spec coverage:** §A GET /sessions → Task 1; §B.1 Frame → Task 3; §B.2 NetSource (connect/current/subscribe → Task 3; send/prose/done/history → Task 4; reconnect/auth → Task 5); §B.3 sessionsClient → Task 2; §C.1 SessionSelectScreen → Task 6; §C.2 SessionScreen container → Task 7; §C.3 wiring/store → Task 7. Error handling (§5) distributed across Tasks 4–5,7. Covered.

**Placeholder scan:** Task 5's auth-refresh step describes the mechanism + names the test but shows less literal code than others — the executor implements the `error`-401/`onerror` → `getToken`→reconnect→`onAuthLost` path per the described state (`authRetried` flag). Acceptable (behavior + test named); if the executor needs more, the pattern mirrors the reconnect code shown. All other steps carry literal code.

**Type consistency:** `Frame` shape identical across Tasks 3–4. `NetSourceDeps` (chatId/wsUrl/getToken/onAuthLost/makeSocket) stable. `Intent` uses `'token'`/`'free'` (matches intents.ts) — send maps free→`{text}`. `SessionSummary{chatId,caseId,seed,createdAt}` consistent Tasks 2/6. Server `SessionsByUser` + `CreatedAt` consistent Task 1. `wsUrlFrom` defined Task 7, used there.

## Границы (не здесь)
Свободный NL-текст (сервер без парсера); мультиплеер/канал user; outbox/offline; оптимистичный UI; удаление/переименование сессий; выбор дела кроме harbour.
