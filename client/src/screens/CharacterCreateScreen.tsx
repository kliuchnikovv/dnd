import React, { useState } from 'react';
import { Pressable, ScrollView, StyleSheet, TextInput, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Button, Card, Text } from '@genie/ds';
import { useTheme } from '../theme/ThemeContext';
import { Archetype, RulesetCreation, dnd5eCreation, rulesets } from '../mocks/app';
import { Ruleset, useCharacters } from '../state/characters';

// Создание персонажа (14a, визард) — тёплый онбординг, не «лист D&D». Теперь трёхшаговый:
// сперва правила (thresholdCreation / dnd5eCreation из rulesets — mocks/app.ts), затем архетип
// из каталога выбранного правила, затем имя. Сохраняем через useCharacters().add — персонаж
// оседает в локальном сторе (zustand + async-storage), а не только в этом экране.
export const CharacterCreateScreen: React.FC = () => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    const add = useCharacters((s) => s.add);

    const [pickedRuleset, setPickedRuleset] = useState<RulesetCreation | null>(null);
    const [pickedArchetype, setPickedArchetype] = useState<string | null>(null);
    const [name, setName] = useState('');
    const [saving, setSaving] = useState(false);

    // step выводится из выбора, а не хранится отдельно — картой нельзя провалиться
    // в рассинхрон «шаг сам по себе, выбор сам по себе».
    const step = pickedRuleset === null ? 0 : pickedArchetype === null ? 1 : 2;

    const back = () => {
        if (step === 2) setPickedArchetype(null);
        else if (step === 1) setPickedRuleset(null);
    };

    const onSave = async () => {
        if (!pickedRuleset || !pickedArchetype || name.trim().length === 0) return;
        setSaving(true);
        try {
            // RulesetCreation не несёт машинный код правила (только витринное ruleName) —
            // код выводим по ссылке на сам объект каталога, без парсинга текста.
            const rulesetCode: Ruleset = pickedRuleset === dnd5eCreation ? 'dnd5e' : 'threshold';
            await add(name.trim(), rulesetCode, pickedArchetype);
        } finally {
            setSaving(false);
        }
    };

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
                            По каким правилам играем?
                        </Text>
                        {rulesets.map((r) => (
                            <RulesetCard key={r.ruleName} ruleset={r} onPick={() => setPickedRuleset(r)} />
                        ))}
                    </>
                ) : null}

                {step === 1 && pickedRuleset ? (
                    <>
                        <Text theme={theme} variant="title" style={{ color: c.text }}>
                            Кто вы в этом порту?
                        </Text>
                        {pickedRuleset.archetypes.map((a) => (
                            <ArchetypeCard
                                key={a.id}
                                archetype={a}
                                selected={pickedArchetype === a.id}
                                onPick={() => setPickedArchetype(a.id)}
                            />
                        ))}
                    </>
                ) : null}

                {step === 2 ? (
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
            </ScrollView>

            {step > 0 ? (
                <View style={[styles.footer, { backgroundColor: c.background, borderTopColor: c.hairline, paddingBottom: insets.bottom + 8 }]}>
                    <Button theme={theme} label="назад" variant="secondary" style={styles.backBtn} onPress={back} />
                    {step === 2 ? (
                        <Button
                            theme={theme}
                            label="создать"
                            variant="primary"
                            style={styles.saveBtn}
                            loading={saving}
                            disabled={name.trim().length === 0}
                            onPress={onSave}
                        />
                    ) : null}
                </View>
            ) : null}
        </View>
    );
};

const RulesetCard: React.FC<{ ruleset: RulesetCreation; onPick: () => void }> = ({ ruleset, onPick }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    return (
        <Pressable onPress={onPick}>
            <Card theme={theme} surface="surface" radius="lg" bordered style={[styles.arch, { borderColor: c.hairline }]}>
                <Text theme={theme} variant="title" style={{ color: c.text }}>
                    {ruleset.ruleName}
                </Text>
                <Text theme={theme} variant="body" style={[styles.archBlurb, { color: c.inkSecondary }]}>
                    {ruleset.archetypes.length} архетипа · бюджет статов {ruleset.statBudget}, максимум {ruleset.statMax}
                </Text>
            </Card>
        </Pressable>
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
    footer: { flexDirection: 'row', gap: 10, paddingHorizontal: 18, paddingTop: 10, borderTopWidth: 1 },
    backBtn: { flex: 1 },
    saveBtn: { flex: 2 },
});
