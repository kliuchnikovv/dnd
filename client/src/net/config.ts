// Базовый URL сервера. Из EXPO_PUBLIC_API_URL (доступна в рантайме Expo),
// иначе — локальный дефолт для разработки. Секретов тут нет.
export function apiBaseUrl(): string {
  const fromEnv = process.env.EXPO_PUBLIC_API_URL;
  return (fromEnv && fromEnv.replace(/\/+$/, '')) || 'http://localhost:8080';
}
