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
