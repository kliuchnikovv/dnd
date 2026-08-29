import React, { useState } from 'react';
import { Pressable, ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Card, Text } from '@genie/ds';
import { useTheme } from '../theme/ThemeContext';
import { Avatar, NarrationFeed, OptionsRail, ResolutionCard, Composer } from '../components/turnview';
import { Icon } from '../components/Icon';
import { TurnView, Who } from '../turnview/types';
import { Intent } from '../turnview/intents';
import { dispositionTone } from '../components/turnview/tone';

// Диалог (10a, гибрид) — на машинерии чата: шапка с аватаром/именем/расположением, пузыри NPC
// слева, реплики игрока справа БЕЗ подписи/аватара (правило контента), варианты списком над вводом.

function dispositionWord(d: number): string {
    if (d > 0) return 'расположен';
    if (d < 0) return 'настороже';
    return 'нейтрален';
}

export const DialogueScreen: React.FC<{
    view: TurnView;
    onIntent: (i: Intent) => void;
    onExit?: () => void;
}> = ({ view, onIntent, onExit }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    const [echoes, setEchoes] = useState<string[]>([]);

    const npc: Who | undefined = (view.participants ?? []).find((w) => w.id !== 'pc');

    const handleIntent = (intent: Intent) => {
        // Локальное эхо реплики игрока (в моке сервер не возвращает её обратно).
        if (intent.kind === 'free') {
            setEchoes((e) => [...e, intent.text]);
        } else {
            const opt = (view.options ?? []).find((o) => o.token === intent.token);
            if (opt && opt.reply) setEchoes((e) => [...e, opt.label]);
        }
        onIntent(intent);
    };

    return (
        <View style={[styles.root, { backgroundColor: c.background }]}>
            <View style={[styles.header, { paddingTop: insets.top + 8, borderBottomColor: c.hairline }]}>
                <Pressable onPress={onExit} hitSlop={10} style={styles.back}>
                    <Icon name="back" size={22} color={c.inkMuted ?? c.textSecondary} />
                </Pressable>
                {npc ? <Avatar who={npc} size={34} /> : null}
                <View style={styles.headTitles}>
                    <Text theme={theme} variant="title" style={{ color: c.text }}>
                        {npc?.name ?? 'Разговор'}
                    </Text>
                    {npc ? (
                        <Text theme={theme} variant="mono" style={[styles.disp, { color: dispositionTone(npc.disposition, c) }]}>
                            {dispositionWord(npc.disposition).toUpperCase()}
                        </Text>
                    ) : null}
                </View>
            </View>

            <ScrollView
                contentContainerStyle={styles.thread}
                showsVerticalScrollIndicator={false}
            >
                <ResolutionCard resolution={view.resolution} />
                <NarrationFeed narration={view.narration} />
                {echoes.map((line, i) => (
                    <Card
                        key={i}
                        theme={theme}
                        surface="surfaceHighlight"
                        radius="lg"
                        style={[styles.playerBubble]}
                    >
                        <Text theme={theme} variant="body" style={{ color: c.text }}>
                            {line}
                        </Text>
                    </Card>
                ))}
            </ScrollView>

            <View style={[styles.dock, { backgroundColor: c.background, borderTopColor: c.hairline, paddingBottom: insets.bottom + 8 }]}>
                <OptionsRail options={view.options} onIntent={handleIntent} layout="list" />
                <Composer onIntent={handleIntent} placeholder="ответить…" />
            </View>
        </View>
    );
};

const styles = StyleSheet.create({
    root: { flex: 1 },
    header: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 10,
        paddingHorizontal: 16,
        paddingBottom: 10,
        borderBottomWidth: 1,
    },
    back: { padding: 2 },
    headTitles: { gap: 2 },
    disp: { fontSize: 9.5, letterSpacing: 1.2 },
    thread: { paddingHorizontal: 18, paddingVertical: 14, gap: 12 },
    playerBubble: { alignSelf: 'flex-end', maxWidth: '82%' },
    dock: { paddingHorizontal: 18, paddingTop: 10, borderTopWidth: 1, gap: 10 },
});
