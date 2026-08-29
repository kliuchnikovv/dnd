import React, { useState } from 'react';
import { Pressable, ScrollView, StyleSheet, TextInput, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Button, Card, Text } from '@genie/ds';
import { useTheme } from '../theme/ThemeContext';
import { Archetype, RulesetCreation, thresholdCreation } from '../mocks/app';

// Создание персонажа (14a, визард) — тёплый онбординг, не «лист D&D». Ruleset-driven: карточки
// архетипов и статы приходят дескрипторами из правила «Порог» (label от правила, не хардкод).
export const CharacterCreateScreen: React.FC<{ ruleset?: RulesetCreation }> = ({ ruleset = thresholdCreation }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    const [step, setStep] = useState(0);
    const [picked, setPicked] = useState<string | undefined>();
    const [name, setName] = useState('');

    const canNext = step === 0 ? picked !== undefined : step === 1 ? name.trim().length > 0 : true;

    return (
        <View style={[styles.root, { backgroundColor: c.background }]}>
            <ScrollView
                contentContainerStyle={[styles.content, { paddingTop: insets.top + 16, paddingBottom: insets.bottom + 120 }]}
                showsVerticalScrollIndicator={false}
            >
                <View style={styles.progress}>
                    {[0, 1, 2].map((i) => (
                        <View
                            key={i}
                            style={[styles.segment, { backgroundColor: i <= step ? c.accent : c.stroke ?? c.border }]}
                        />
                    ))}
                </View>
                <Text theme={theme} variant="mono" style={[styles.stepLabel, { color: c.inkMuted }]}>
                    ШАГ {step + 1} ИЗ 3
                </Text>

                {step === 0 ? (
                    <>
                        <Text theme={theme} variant="title" style={{ color: c.text }}>
                            Кто вы в этом порту?
                        </Text>
                        {ruleset.archetypes.map((a) => (
                            <ArchetypeCard key={a.id} archetype={a} selected={picked === a.id} onPick={() => setPicked(a.id)} />
                        ))}
                    </>
                ) : null}

                {step === 1 ? (
                    <>
                        <Text theme={theme} variant="title" style={{ color: c.text }}>
                            Как вас звать?
                        </Text>
                        <TextInput
                            value={name}
                            onChangeText={setName}
                            placeholder="имя дознавателя"
                            placeholderTextColor={c.inkFaint}
                            style={[styles.nameInput, { borderColor: c.stroke ?? c.border, color: c.text, backgroundColor: c.surface, fontFamily: 'PTSerif_400Regular' }]}
                        />
                    </>
                ) : null}

                {step === 2 ? (
                    <View style={styles.done}>
                        <Text theme={theme} variant="title" style={{ color: c.text }}>
                            {name.trim()} · {ruleset.archetypes.find((a) => a.id === picked)?.name}
                        </Text>
                        <Text theme={theme} variant="body" style={[styles.doneNote, { color: c.inkSecondary }]}>
                            Магистрат внёс вас в списки. Первое дело уже ждёт на пристани.
                        </Text>
                    </View>
                ) : null}
            </ScrollView>

            <View style={[styles.footer, { backgroundColor: c.background, borderTopColor: c.hairline, paddingBottom: insets.bottom + 8 }]}>
                <Button
                    theme={theme}
                    label={step < 2 ? 'дальше' : 'создать'}
                    variant="primary"
                    fullWidth
                    disabled={!canNext}
                    onPress={() => setStep((s) => Math.min(2, s + 1))}
                />
            </View>
        </View>
    );
};

const ArchetypeCard: React.FC<{ archetype: Archetype; selected: boolean; onPick: () => void }> = ({ archetype, selected, onPick }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    return (
        <Pressable onPress={onPick}>
            <Card
                theme={theme}
                surface={selected ? 'surfaceHighlight' : 'surface'}
                radius="lg"
                bordered
                style={[styles.arch, { borderColor: selected ? c.accent : c.hairline }]}
            >
                <View style={styles.archHead}>
                    <Text theme={theme} variant="title" style={{ color: c.text }}>
                        {archetype.name}
                    </Text>
                    {selected ? (
                        <Text theme={theme} variant="mono" style={{ color: c.amberText }}>
                            ✓
                        </Text>
                    ) : null}
                </View>
                <Text theme={theme} variant="body" style={[styles.archBlurb, { color: c.inkSecondary }]}>
                    {archetype.blurb}
                </Text>
                <View style={styles.archMeta}>
                    {archetype.stats.map((s) => (
                        <Text key={s.label} theme={theme} variant="mono" style={[styles.stat, { color: c.inkMuted }]}>
                            {s.label.toUpperCase()} +{s.value}
                        </Text>
                    ))}
                </View>
                <View style={styles.tags}>
                    {archetype.tags.map((t) => (
                        <View key={t} style={[styles.tag, { backgroundColor: c.amberFill }]}>
                            <Text theme={theme} variant="mono" style={[styles.tagText, { color: c.amberText }]}>
                                {t}
                            </Text>
                        </View>
                    ))}
                </View>
            </Card>
        </Pressable>
    );
};

const styles = StyleSheet.create({
    root: { flex: 1 },
    content: { paddingHorizontal: 18, gap: 12 },
    progress: { flexDirection: 'row', gap: 6 },
    segment: { flex: 1, height: 4, borderRadius: 999 },
    stepLabel: { fontSize: 9.5, letterSpacing: 1.2 },
    arch: { gap: 8 },
    archHead: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
    archBlurb: { fontStyle: 'italic' },
    archMeta: { flexDirection: 'row', flexWrap: 'wrap', gap: 10 },
    stat: { fontSize: 10, letterSpacing: 0.6 },
    tags: { flexDirection: 'row', flexWrap: 'wrap', gap: 6 },
    tag: { borderRadius: 999, paddingVertical: 3, paddingHorizontal: 9 },
    tagText: { fontSize: 9, letterSpacing: 0.4 },
    nameInput: { borderWidth: 1, borderRadius: 14, paddingHorizontal: 14, paddingVertical: 12, fontSize: 16 },
    done: { gap: 10 },
    doneNote: { fontStyle: 'italic' },
    footer: { paddingHorizontal: 18, paddingTop: 10, borderTopWidth: 1 },
});
