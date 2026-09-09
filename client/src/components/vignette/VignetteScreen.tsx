import React, { useEffect, useMemo, useState } from 'react';
import { ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Card, Spinner, Text } from '@genie/ds';
import { useTheme } from '../../theme/ThemeContext';
import { NarrationFeed, Composer } from '../turnview';
import { VignetteNetSource, wsUrlFrom } from '../../net/vignetteNetSource';
import { apiBaseUrl } from '../../net/config';
import { getSocketToken, onSocketAuthLost } from '../../net/socketToken';
import { useVignetteView } from '../../vignette/useVignetteView';

// VignetteScreen — контейнер + рендер виньетка-сессии, ЦЕЛИКОМ отдельный от
// screens/session/SessionScreen.tsx (container) + screens/SessionScreen.tsx
// (mode-router) + AppShell: не переиспользует TurnView-рендереры (SceneScreen/
// DialogueScreen/MapScreen/DossierScreen), только два УЖЕ агностичных куска —
// NarrationFeed и Composer (см. docs/handoff/2026-09-06-
// vignette-turnview-projection.md §2 — «то, что действительно даром»).
//
// КУДА ПОДКЛЮЧИТЬ (не сделано этим коммитом — см. отчёт задачи):
//   client/App.tsx сегодня всегда рендерит <SessionScreen chatId={chatId}/>
//   (session/SessionScreen.tsx). Нужно завести рядом с chatId состояние kind
//   (screens/session/sessionKind.ts: 'turn' | 'vignette'), выставлять его в
//   SessionSelectScreen.onPick (для нового дела — из pickedCase.rules через
//   sessionKindOf(); для resume-строки — kindForResumedSession(row.caseId,
//   cases)), и в App.tsx ветвиться:
//     kind === 'vignette' ? <VignetteScreen chatId={chatId}/> : <SessionScreen chatId={chatId}/>
//   SessionSelectScreen.tsx/App.tsx НЕ правились — эта правка на усмотрение
//   того, кто подключает (сигнатуры onPick/onStart придётся расширить третьим
//   параметром или объектом {chatId,kind}).
export const VignetteScreen: React.FC<{ chatId: string }> = ({ chatId }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    const [connectError, setConnectError] = useState<string | null>(null);

    const net = useMemo(
        () => new VignetteNetSource({ chatId, wsUrl: wsUrlFrom(apiBaseUrl()), getToken: getSocketToken, onAuthLost: onSocketAuthLost }),
        [chatId],
    );

    useEffect(() => {
        setConnectError(null);
        net.connect().catch((e) => setConnectError((e as Error).message));
        return () => net.close();
    }, [net]);

    const { snapshot, send } = useVignetteView(net);
    const { mechanics, narration, showEnded } = snapshot;

    if (mechanics.scene === '' && narration.length === 0) {
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

    const surfaces = mechanics.surfaces ?? [];
    const revealed = mechanics.revealed ?? [];

    return (
        <View style={[styles.root, { backgroundColor: c.background }]}>
            <View style={[styles.header, { paddingTop: insets.top + 48, borderBottomColor: c.hairline, backgroundColor: c.background }]}>
                <Text theme={theme} variant="title" style={{ color: c.text }}>
                    {mechanics.scene}
                </Text>
                {mechanics.state_note ? (
                    <Text theme={theme} variant="caption" style={{ color: c.inkMuted, fontStyle: 'italic' }}>
                        {mechanics.state_note}
                    </Text>
                ) : null}
                {/* beat — только ЭТОГО хода (сервер шлёт его лишь в session_state
                    хода, где эскалация случилась; следующий снимок его просто не
                    несёт — omitempty), баннер сам гаснет со следующим ходом. */}
                {mechanics.beat ? (
                    <View style={[styles.beat, { borderColor: c.accent }]}>
                        <Text theme={theme} variant="mono" style={{ color: c.accent, fontSize: 11 }}>
                            {mechanics.beat}
                        </Text>
                    </View>
                ) : null}
            </View>

            <ScrollView style={styles.feed} contentContainerStyle={styles.content} showsVerticalScrollIndicator={false}>
                {surfaces.length > 0 ? (
                    <Card theme={theme} surface="surface" radius="lg" bordered style={[styles.card, { borderColor: c.hairline }]}>
                        <Text theme={theme} variant="mono" style={[styles.cardLabel, { color: c.inkMuted }]}>
                            ЗДЕСЬ
                        </Text>
                        {surfaces.map((s, i) => (
                            <Text key={i} theme={theme} variant="body" style={{ color: c.text }}>
                                {s}
                            </Text>
                        ))}
                    </Card>
                ) : null}

                <NarrationFeed narration={narration} />

                {/* revealed — тихий, второстепенный список: проза Мастера обычно
                    уже проговаривает то же самое (см. хендофф §3, строка
                    «revealed» — «избыточно»). Оставлен как подстраховка на
                    случай, если страж вырежет упоминание из прозы, но не как
                    основная подача. */}
                {revealed.length > 0 ? (
                    <View style={styles.revealedBlock}>
                        <Text theme={theme} variant="mono" style={[styles.cardLabel, { color: c.inkFaint }]}>
                            ЗАМЕЧЕНО
                        </Text>
                        {revealed.map((r, i) => (
                            <Text key={i} theme={theme} variant="caption" style={{ color: c.inkFaint }}>
                                {r}
                            </Text>
                        ))}
                    </View>
                ) : null}

                {showEnded ? (
                    <Card theme={theme} surface="surface" radius="lg" bordered style={[styles.endCard, { borderColor: c.accent }]}>
                        <Text theme={theme} variant="title" style={{ color: c.text }}>
                            Конец
                        </Text>
                        <Text theme={theme} variant="body" style={{ color: c.inkSecondary }}>
                            {mechanics.end_text ?? ''}
                        </Text>
                    </Card>
                ) : null}
            </ScrollView>

            {!showEnded ? (
                <View style={[styles.dock, { backgroundColor: c.background, borderTopColor: c.hairline, paddingBottom: insets.bottom + 8 }]}>
                    <Composer onIntent={(intent) => intent.kind === 'free' && send(intent.text)} />
                </View>
            ) : null}
        </View>
    );
};

const styles = StyleSheet.create({
    root: { flex: 1 },
    center: { flex: 1, alignItems: 'center', justifyContent: 'center' },
    error: { textAlign: 'center', marginTop: 8, paddingHorizontal: 24 },
    header: { paddingHorizontal: 18, paddingBottom: 12, gap: 6, borderBottomWidth: 1 },
    beat: { alignSelf: 'flex-start', borderWidth: 1, borderRadius: 999, paddingVertical: 3, paddingHorizontal: 10, marginTop: 4 },
    feed: { flex: 1 },
    content: { paddingHorizontal: 18, paddingTop: 14, paddingBottom: 20, gap: 14 },
    card: { gap: 6 },
    cardLabel: { fontSize: 9.5, letterSpacing: 1.4 },
    revealedBlock: { gap: 2 },
    endCard: { gap: 8, marginTop: 8 },
    dock: { paddingHorizontal: 18, paddingTop: 10, borderTopWidth: 1, gap: 10 },
});
