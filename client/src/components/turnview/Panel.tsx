import React from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import { Card, Text } from '@genie/ds';
import { useTheme } from '../../theme/ThemeContext';
import { Panel as PanelData, PanelItem, Section, Slot } from '../../turnview/types';
import { Intent, tokenIntent } from '../../turnview/intents';
import { listKey } from './keys';

// Panel — обобщённая панель цели от СЦЕНАРИЯ (kind "deduction" — досье, но клиент этого слова
// не знает: рисует секции, факты и слоты по данным). Анти-спойлер: ничего не красим «верным/
// зелёным». Корроборация (ведущая «*» в sub) — про крепость, НЕ про правоту. Тап по item/опции
// с token шлёт его как есть.

export const Panel: React.FC<{ panel?: PanelData; onIntent: (intent: Intent) => void }> = ({ panel, onIntent }) => {
    const { descriptor: theme } = useTheme();
    if (!panel || !panel.surface) return null;
    const c = theme.colors;

    return (
        <View style={styles.wrap}>
            <Text theme={theme} variant="title" style={{ color: c.text }}>
                {panel.title}
            </Text>
            {(panel.sections ?? []).map((s, i) => (
                <SectionView key={`${s.label}-${i}`} section={s} onIntent={onIntent} />
            ))}
        </View>
    );
};

const SectionView: React.FC<{ section: Section; onIntent: (intent: Intent) => void }> = ({ section, onIntent }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    return (
        <View style={styles.section}>
            <Text theme={theme} variant="mono" style={[styles.sectionLabel, { color: c.inkMuted }]}>
                {section.label.toUpperCase()}
            </Text>
            {(section.items ?? []).map((it, i) => (
                <FactRow key={listKey(it.id, i)} item={it} onIntent={onIntent} />
            ))}
            {(section.slots ?? []).length > 0 ? (
                <View style={styles.slotsGrid}>
                    {section.slots!.map((sl, i) => (
                        <SlotCard key={listKey(sl.name, i)} slot={sl} onIntent={onIntent} />
                    ))}
                </View>
            ) : null}
        </View>
    );
};

const FactRow: React.FC<{ item: PanelItem; onIntent: (intent: Intent) => void }> = ({ item, onIntent }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const corroborated = (item.sub ?? '').trimStart().startsWith('*');
    const body = (
        <Card theme={theme} surface="surface" radius="lg" bordered style={[styles.factCard, { borderColor: c.hairline }]}>
            <View style={styles.factHead}>
                <Text theme={theme} variant="body" style={[styles.factLabel, { color: c.text }]}>
                    {item.label}
                </Text>
                {corroborated ? (
                    <Text theme={theme} variant="mono" style={{ color: c.moss }}>
                        ✓
                    </Text>
                ) : null}
            </View>
            {item.sub ? (
                <Text theme={theme} variant="mono" style={[styles.factSub, { color: c.inkFaint }]}>
                    {item.sub}
                </Text>
            ) : null}
        </Card>
    );
    if (item.token) {
        return (
            <Pressable onPress={() => onIntent(tokenIntent(item.token!))} hitSlop={6}>
                {body}
            </Pressable>
        );
    }
    return body;
};

const SlotCard: React.FC<{ slot: Slot; onIntent: (intent: Intent) => void }> = ({ slot, onIntent }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const options = slot.options ?? [];
    const locked = !slot.filled && options.length === 0;

    return (
        <Card
            theme={theme}
            surface={slot.filled ? 'surfaceHighlight' : 'surface'}
            radius="lg"
            bordered
            style={[
                styles.slotCard,
                {
                    borderColor: slot.filled ? c.accent : c.stroke ?? c.border,
                    borderStyle: locked ? 'dashed' : 'solid',
                    opacity: locked ? 0.6 : 1,
                },
            ]}
        >
            <Text theme={theme} variant="mono" style={[styles.slotLabel, { color: c.inkMuted }]}>
                {slot.label.toUpperCase()}
            </Text>
            {slot.filled ? (
                <Text theme={theme} variant="body" style={{ color: c.text }}>
                    {slot.filled.label}
                </Text>
            ) : locked ? (
                <Text theme={theme} variant="mono" style={[styles.slotHint, { color: c.inkFaint }]}>
                    нет открытых токенов
                </Text>
            ) : (
                <View style={styles.slotOptions}>
                    {options.map((o, i) => (
                        <Pressable
                            key={listKey(o.id, i)}
                            onPress={() => (o.token ? onIntent(tokenIntent(o.token)) : undefined)}
                            hitSlop={6}
                            style={({ pressed }) => [
                                styles.slotOption,
                                { borderColor: c.stroke ?? c.border, opacity: pressed ? 0.8 : 1 },
                            ]}
                        >
                            <Text theme={theme} variant="caption" style={{ color: c.amberText }}>
                                {o.label}
                            </Text>
                        </Pressable>
                    ))}
                </View>
            )}
        </Card>
    );
};

const styles = StyleSheet.create({
    wrap: { gap: 16 },
    section: { gap: 8 },
    sectionLabel: { fontSize: 9.5, letterSpacing: 1.4 },
    factCard: { gap: 4 },
    factHead: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', gap: 8 },
    factLabel: { flexShrink: 1 },
    factSub: { fontSize: 10 },
    slotsGrid: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
    slotCard: { width: '48%', gap: 6, minHeight: 74 },
    slotLabel: { fontSize: 9, letterSpacing: 1.2 },
    slotHint: { fontSize: 10 },
    slotOptions: { flexDirection: 'row', flexWrap: 'wrap', gap: 6 },
    slotOption: { borderWidth: 1, borderRadius: 999, paddingVertical: 4, paddingHorizontal: 10 },
});
