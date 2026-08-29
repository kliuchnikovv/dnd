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
