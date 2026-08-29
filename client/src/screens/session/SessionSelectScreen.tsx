import React, { useCallback, useEffect, useState } from 'react';
import { ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Button, ListItem, Spinner, Text } from '@genie/ds';
import { useTheme } from '../../theme/ThemeContext';
import { useAuth } from '../../state/useAuth';
import { secureTokenStore } from '../../auth/tokenStore';
import { listSessions, createSession } from '../../net/sessionsClient';
import { toSessionRows, SessionRow } from './sessionSelect';

// SessionSelectScreen — выбор существующей сессии или старт новой. Вся vm-
// логика (маппинг в строки, относительное время) — в sessionSelect.ts и
// юнит-тестируется отдельно; экран лишь грузит список, рендерит и зовёт
// onPick(chatId) по тапу на строку или после создания новой игры.
//
// useAuth() отдаёт только status/user/error — сам access-токен наружу не
// торчит (см. AuthState), он лежит в secureTokenStore, тем же хранилищем,
// которым пользуются restore()/refresh() внутри useAuth.ts. Экран читает его
// оттуда напрямую вместо расширения публичного интерфейса стора.
export const SessionSelectScreen: React.FC<{ onPick: (chatId: string) => void }> = ({ onPick }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    const { status } = useAuth();

    const [rows, setRows] = useState<SessionRow[]>([]);
    const [loading, setLoading] = useState(true);
    const [creating, setCreating] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const getToken = useCallback(async (): Promise<string | null> => {
        const t = await secureTokenStore.load();
        return t ? t.accessToken : null;
    }, []);

    const load = useCallback(async () => {
        setLoading(true);
        setError(null);
        try {
            const token = await getToken();
            if (!token) {
                setRows([]);
                return;
            }
            const list = await listSessions(token);
            setRows(toSessionRows(list, Math.floor(Date.now() / 1000)));
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setLoading(false);
        }
    }, [getToken]);

    useEffect(() => {
        if (status === 'signedIn') {
            void load();
        } else {
            setLoading(false);
        }
    }, [status, load]);

    const onNewGame = useCallback(async () => {
        const token = await getToken();
        if (!token) {
            setError('нет токена — войдите заново');
            return;
        }
        setCreating(true);
        setError(null);
        try {
            const { chatId } = await createSession(token, 'harbour');
            onPick(chatId);
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setCreating(false);
        }
    }, [getToken, onPick]);

    return (
        <View style={[styles.root, { backgroundColor: c.background, paddingTop: insets.top + 24, paddingBottom: insets.bottom + 24 }]}>
            <Text theme={theme} variant="display" style={{ color: c.text }}>
                Твои дела
            </Text>

            {loading ? (
                <View style={styles.center}>
                    <Spinner theme={theme} size="lg" />
                </View>
            ) : (
                <ScrollView style={styles.list} showsVerticalScrollIndicator={false}>
                    {rows.map((row) => (
                        <ListItem
                            key={row.chatId}
                            theme={theme}
                            title={row.caseId}
                            subtitle={row.subtitle}
                            onPress={() => onPick(row.chatId)}
                        />
                    ))}
                </ScrollView>
            )}

            {error ? (
                <Text theme={theme} variant="caption" tone="error" style={styles.error}>
                    {error}
                </Text>
            ) : null}

            <Button
                theme={theme}
                label="Новая игра"
                variant="primary"
                size="lg"
                fullWidth
                loading={creating}
                onPress={onNewGame}
            />
        </View>
    );
};

const styles = StyleSheet.create({
    root: {
        flex: 1,
        paddingHorizontal: 24,
        gap: 16,
    },
    center: {
        flex: 1,
        alignItems: 'center',
        justifyContent: 'center',
    },
    list: {
        flex: 1,
    },
    error: {
        textAlign: 'center',
    },
});
