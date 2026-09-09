import { secureTokenStore } from '../auth/tokenStore';
import { authStore } from '../state/useAuth';

// getSocketToken/onSocketAuthLost — вынесено из screens/session/SessionScreen.tsx
// (там были module-level getToken()/onAuthLost(), не экспортированы) в общий
// файл, ЧТОБЫ VignetteNetSource-контейнер (components/vignette/VignetteScreen.tsx)
// не плодил третью копию той же логики токен-рефреша. screens/session/
// SessionScreen.tsx НЕ ПРАВИЛСЯ (по заданию — только новые файлы): его
// собственные getToken()/onAuthLost() остались как есть и сегодня дублируют
// это один-в-один. Если когда-нибудь захочется убрать дублирование — там
// достаточно заменить локальные функции на импорт отсюда, поведение то же
// самое (проверено побайтовым сравнением исходников, НЕ прогоном).

const REFRESH_MARGIN_MS = 60_000;

export async function getSocketToken(): Promise<string> {
    const t = await secureTokenStore.load();
    if (!t) throw new Error('нет токена');
    if (t.expiresAt * 1000 - Date.now() < REFRESH_MARGIN_MS) {
        await authStore.getState().refresh();
        const nt = await secureTokenStore.load();
        if (!nt) throw new Error('рефреш не дал токена');
        return nt.accessToken;
    }
    return t.accessToken;
}

export function onSocketAuthLost(): void {
    void authStore.getState().logout();
}
