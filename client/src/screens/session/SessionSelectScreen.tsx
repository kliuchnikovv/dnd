import React, { useCallback, useEffect, useState } from 'react';
import { ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Button, Chip, ListItem, Spinner, Text } from '@genie/ds';
import { useTheme } from '../../theme/ThemeContext';
import { useAuth } from '../../state/useAuth';
import { secureTokenStore } from '../../auth/tokenStore';
import { listSessions, createSession } from '../../net/sessionsClient';
import { listCases, CaseSummary } from '../../net/cases';
import { CharacterRecord, useCharacters } from '../../state/characters';
import { toSessionRows, SessionRow } from './sessionSelect';
import { SessionKind, sessionKindOf, kindForResumedSession } from './sessionKind';

// SessionPick — что открывать в App.tsx после выбора: chatId + kind. Kind
// нужен ДО первого WS-кадра (см. sessionKind.ts): по нему App.tsx решает,
// монтировать TurnView (SessionScreen) или VignetteScreen. Экспортируется,
// чтобы App.tsx мог типизировать состояние.
export type SessionPick = { chatId: string; kind: SessionKind };

// SessionSelectScreen — продолжить старое дело или начать новое. Старый
// список сессий (resume по chatId) остаётся как был; новизна — старт нового
// дела теперь двухшаговый: «Кто идёт?» (персонаж из локального стора, Task 7)
// → «Куда?» (дела из GET /cases, отфильтрованные по совпадению
// character.ruleset === case.rules — персонаж-D&D5e не полезет в дело под
// «Порог», и наоборот). useAuth() отдаёт только status/user/error — сам
// access-токен наружу не торчит (см. AuthState), он лежит в
// secureTokenStore, тем же хранилищем, которым пользуются restore()/refresh()
// внутри useAuth.ts. Экран читает его оттуда напрямую вместо расширения
// публичного интерфейса стора.
export const SessionSelectScreen: React.FC<{ onPick: (p: SessionPick) => void }> = ({ onPick }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    const { status } = useAuth();
    const characters = useCharacters((s) => s.characters);

    const [rows, setRows] = useState<SessionRow[]>([]);
    const [cases, setCases] = useState<CaseSummary[]>([]);
    const [loading, setLoading] = useState(true);
    const [pickedCharacter, setPickedCharacter] = useState<CharacterRecord | null>(null);
    const [pickedCase, setPickedCase] = useState<CaseSummary | null>(null);
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
                setCases([]);
                return;
            }
            const [sessions, caseList] = await Promise.all([listSessions(token), listCases(token)]);
            setRows(toSessionRows(sessions, Math.floor(Date.now() / 1000)));
            setCases(caseList);
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

    // Совместимые дела — только те, чья система правил совпадает с ruleset'ом
    // выбранного персонажа. Несовместимые просто не показываем: список и так
    // короткий, а "серый недоступный пункт" не добавляет ценности здесь.
    const compatibleCases = pickedCharacter ? cases.filter((cs) => cs.rules === pickedCharacter.ruleset) : [];

    const pickCharacter = useCallback((ch: CharacterRecord) => {
        setPickedCharacter(ch);
        setPickedCase(null); // прошлый выбор дела мог быть несовместим с новым персонажем
    }, []);

    const onStart = useCallback(async () => {
        if (!pickedCharacter || !pickedCase) return;
        const token = await getToken();
        if (!token) {
            setError('нет токена — войдите заново');
            return;
        }
        setCreating(true);
        setError(null);
        try {
            const { chatId } = await createSession(token, pickedCase.id, pickedCharacter.id);
            onPick({ chatId, kind: sessionKindOf(pickedCase) });
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setCreating(false);
        }
    }, [getToken, onPick, pickedCase, pickedCharacter]);

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
                            onPress={() => onPick({ chatId: row.chatId, kind: kindForResumedSession(row.caseId, cases) })}
                        />
                    ))}

                    <Text theme={theme} variant="title" style={[styles.sectionTitle, { color: c.text }]}>
                        Новое дело
                    </Text>
                    <Text theme={theme} variant="mono" style={[styles.stepLabel, { color: c.inkMuted }]}>
                        КТО ИДЁТ?
                    </Text>
                    {characters.length === 0 ? (
                        <Text theme={theme} variant="body" style={[styles.hint, { color: c.inkMuted }]}>
                            Персонажей ещё нет — создайте на вкладке «Герой».
                        </Text>
                    ) : (
                        characters.map((ch) => (
                            <ListItem
                                key={ch.id}
                                theme={theme}
                                title={ch.name}
                                selected={pickedCharacter?.id === ch.id}
                                rightContent={<Chip theme={theme} label={ch.ruleset} size="sm" />}
                                onPress={() => pickCharacter(ch)}
                            />
                        ))
                    )}

                    {pickedCharacter ? (
                        <>
                            <Text theme={theme} variant="mono" style={[styles.stepLabel, { color: c.inkMuted }]}>
                                КУДА?
                            </Text>
                            {compatibleCases.length === 0 ? (
                                <Text theme={theme} variant="body" style={[styles.hint, { color: c.inkMuted }]}>
                                    Нет дел под правила «{pickedCharacter.ruleset}».
                                </Text>
                            ) : (
                                compatibleCases.map((cs) => (
                                    <ListItem
                                        key={cs.id}
                                        theme={theme}
                                        title={cs.name}
                                        subtitle={cs.blurb}
                                        selected={pickedCase?.id === cs.id}
                                        onPress={() => setPickedCase(cs)}
                                    />
                                ))
                            )}
                        </>
                    ) : null}
                </ScrollView>
            )}

            {error ? (
                <Text theme={theme} variant="caption" tone="error" style={styles.error}>
                    {error}
                </Text>
            ) : null}

            <Button
                theme={theme}
                label="Начать"
                variant="primary"
                size="lg"
                fullWidth
                loading={creating}
                disabled={!pickedCharacter || !pickedCase}
                onPress={onStart}
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
    sectionTitle: {
        marginTop: 8,
    },
    stepLabel: {
        fontSize: 9.5,
        letterSpacing: 1.2,
    },
    hint: {
        fontStyle: 'italic',
    },
    error: {
        textAlign: 'center',
    },
});
