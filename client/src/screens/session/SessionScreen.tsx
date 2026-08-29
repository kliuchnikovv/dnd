import React, { useEffect, useMemo, useState } from 'react';
import { StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Spinner, Text } from '@genie/ds';
import { useTheme } from '../../theme/ThemeContext';
import { useAppStore } from '../../state/store';
import { useTurnView } from '../../state/useTurnView';
import { AppShell } from '../../navigation/AppShell';
import { NetSource, wsUrlFrom } from '../../net/netSource';
import { apiBaseUrl } from '../../net/config';
import { secureTokenStore } from '../../auth/tokenStore';
import { authStore } from '../../state/useAuth';

// Токен для NetSource: источник правды — secureTokenStore (тот же, которым
// пользуется SessionSelectScreen). Если срок жизни истекает в ближайшую
// минуту — сперва рефреш через authStore, затем перечитать хранилище (§B.2).
const REFRESH_MARGIN_MS = 60_000;

async function getToken(): Promise<string> {
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

function onAuthLost(): void {
    void authStore.getState().logout();
}

// SessionScreen (контейнер) — мост между выбором сессии (Task 6) и игровым
// UI (AppShell + существующий screens/SessionScreen, оба без изменений).
// Строит NetSource на chatId, публикует его как источник вида в общем сторе
// (useAppStore.source — тот же интерфейс TurnViewSource, что и у мока), ждёт
// первого session_state (version > 0) и лишь потом рендерит AppShell.
export const SessionScreen: React.FC<{ chatId: string }> = ({ chatId }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    const setSource = useAppStore((s) => s.setSource);
    const [connectError, setConnectError] = useState<string | null>(null);

    // Стабильно на весь жизненный цикл экрана: пересоздаётся только при смене
    // chatId (переход к другой сессии).
    const net = useMemo(
        () => new NetSource({ chatId, wsUrl: wsUrlFrom(apiBaseUrl()), getToken, onAuthLost }),
        [chatId],
    );

    useEffect(() => {
        setSource(net);
        setConnectError(null);
        net.connect().catch((e) => setConnectError((e as Error).message));
        return () => net.close();
    }, [net, setSource]);

    const { view } = useTurnView(net);

    if (view.version === 0) {
        return (
            <View style={[styles.center, { backgroundColor: c.background, paddingTop: insets.top }]}>
                <Spinner theme={theme} size="lg" />
                <Text theme={theme} variant="caption" style={{ color: c.inkMuted, marginTop: 12 }}>
                    Подключение…
                </Text>
                {connectError ? (
                    <Text theme={theme} variant="caption" tone="error" style={styles.error}>
                        {connectError}
                    </Text>
                ) : null}
            </View>
        );
    }

    return <AppShell />;
};

const styles = StyleSheet.create({
    center: {
        flex: 1,
        alignItems: 'center',
        justifyContent: 'center',
    },
    error: {
        textAlign: 'center',
        marginTop: 8,
        paddingHorizontal: 24,
    },
});
