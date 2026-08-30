// Базовый URL сервера. Из EXPO_PUBLIC_API_URL (доступна в рантайме Expo),
// иначе — локальный дефолт для разработки. Секретов тут нет.
//
// Нормализуем схему: без неё fetch трактует адрес как относительный и
// получается file:///host/path → 404, а wsUrlFrom не распознаёт http(s)://.
// Голый host (railway и т.п.) подразумевает https.
export function apiBaseUrl(): string {
  const fromEnv = process.env.EXPO_PUBLIC_API_URL?.trim();
  if (!fromEnv) return 'http://localhost:8080';
  const withScheme = /^https?:\/\//i.test(fromEnv) ? fromEnv : `https://${fromEnv}`;
  return withScheme.replace(/\/+$/, '');
}
