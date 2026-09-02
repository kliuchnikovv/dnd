import { listCases } from '../cases';

function mockFetch(status: number, body: unknown, capture?: (url: string, opts: any) => void) {
  (global as any).fetch = jest.fn(async (url: string, opts: any) => {
    capture?.(url, opts);
    return { ok: status >= 200 && status < 300, status, json: async () => body };
  });
}

test('listCases GET /cases и шлёт Bearer', async () => {
  let seenUrl = '';
  let seenAuth = '';
  mockFetch(
    200,
    [{ id: 'harbour', name: 'Гавань', rules: 'threshold', scenario: 'investigation', blurb: '...' }],
    (u, o) => {
      seenUrl = u;
      seenAuth = o.headers.Authorization;
    },
  );
  const out = await listCases('tok');
  expect(seenUrl).toMatch(/\/cases$/);
  expect(seenAuth).toBe('Bearer tok');
  expect(out).toEqual([{ id: 'harbour', name: 'Гавань', rules: 'threshold', scenario: 'investigation', blurb: '...' }]);
});

test('non-2xx throws with status', async () => {
  mockFetch(401, { error: 'no' });
  await expect(listCases('tok')).rejects.toMatchObject({ status: 401 });
});
