import React from 'react';
import { StyleSheet, View } from 'react-native';

import { Card, Text } from '@genie/ds';
import { useTheme } from '../../theme/ThemeContext';
import { Block, Resolution } from '../../turnview/types';
import { StreamingCursor } from './StreamingCursor';
import { TypingIndicator } from './TypingIndicator';
import { ResolutionCard } from './ResolutionCard';

// NarrationFeed — проза в контейнере, НЕ разметка: клиент по тексту не навигирует.
// kind различает голос Мастера ("gm", подпись slate) и реплику NPC ("npc", имя amber, курсив).
// Неизвестный kind рендерится как проза Мастера (безопасный дефолт). streaming → курсор.
// Правило контента: прямая речь без тире в начале, курсивом.

export const NarrationFeed: React.FC<{ narration?: Block[] }> = ({ narration }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    if (!narration || narration.length === 0) return null;

    return (
        <View style={styles.feed}>
            {narration.map((b, i) => {
                const isNpc = b.kind === 'npc';
                const isPlayer = b.kind === 'player';
                // Бросок хода — инлайн в ленте (в порядке выполнения), не над ней.
                if (b.kind === 'resolution') {
                    const res = (b as { resolution?: Resolution }).resolution;
                    return <ResolutionCard key={i} resolution={res} />;
                }
                // Проза ещё не пошла (пустой streaming-блок) — «Мастер печатает».
                const isPending = !!b.streaming && b.text.trim() === '';
                if (isPlayer) {
                    // Эхо действия игрока: что я сделал. Отделено от прозы Мастера.
                    return (
                        <View key={i} style={styles.playerBlock}>
                            <Text theme={theme} variant="mono" style={[styles.speaker, { color: c.amberText }]}>
                                ВЫ
                            </Text>
                            <Text theme={theme} variant="body" style={[styles.playerText, { color: c.inkMuted }]}>
                                {b.text}
                            </Text>
                        </View>
                    );
                }
                if (isNpc) {
                    return (
                        <Card
                            key={i}
                            theme={theme}
                            surface="surface"
                            radius="lg"
                            bordered
                            style={[styles.npcCard, { borderColor: c.hairline }]}
                        >
                            {b.speaker ? (
                                <Text theme={theme} variant="mono" style={[styles.speaker, { color: c.amberText }]}>
                                    {b.speaker.name.toUpperCase()}
                                </Text>
                            ) : null}
                            {isPending ? (
                                <TypingIndicator color={c.accent} />
                            ) : (
                                <Text theme={theme} variant="body" style={[styles.reply, { color: c.inkSecondary }]}>
                                    {b.text}
                                    {b.streaming ? <StreamingCursor color={c.accent} /> : null}
                                </Text>
                            )}
                        </Card>
                    );
                }
                // gm и неизвестное — проза Мастера: прозрачный блок с моно-подписью slate.
                return (
                    <View key={i} style={styles.gmBlock}>
                        <Text theme={theme} variant="mono" style={[styles.speaker, { color: c.slate }]}>
                            МАСТЕР
                        </Text>
                        {isPending ? (
                            <TypingIndicator color={c.accent} />
                        ) : (
                            <Text theme={theme} variant="body" style={{ color: c.inkSecondary }}>
                                {b.text}
                                {b.streaming ? <StreamingCursor color={c.accent} /> : null}
                            </Text>
                        )}
                    </View>
                );
            })}
        </View>
    );
};

const styles = StyleSheet.create({
    feed: { gap: 12 },
    gmBlock: { gap: 4 },
    npcCard: { gap: 4 },
    playerBlock: { gap: 2, alignItems: 'flex-end' },
    playerText: { fontStyle: 'italic', textAlign: 'right' },
    speaker: { fontSize: 9.5, letterSpacing: 1.4 },
    reply: { fontStyle: 'italic' },
});
