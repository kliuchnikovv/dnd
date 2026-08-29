import { SessionSummary } from '../../net/sessionsClient';

// Чистая vm-логика экрана выбора сессии: маппинг SessionSummary → SessionRow
// (subtitle = относительное время) и сам relativeTime — вынесены отдельно от
// SessionSelectScreen.tsx, чтобы юнит-тестировать без рендера.
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
