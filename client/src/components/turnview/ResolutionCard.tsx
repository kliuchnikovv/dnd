import React from 'react';
import { StyleSheet, View } from 'react-native';

import { Card, Text } from '@genie/ds';
import { useTheme } from '../../theme/ThemeContext';
import { Resolution } from '../../turnview/types';
import { tierTone } from './tone';

// ResolutionCard — исход броска без словаря конкретной кости. Термы и цель приходят словами
// («внимание +2», «бросок 11» против 12); клиент красит исход по машинной ступени tier,
// но русских слов outcome.label не разбирает.

export const ResolutionCard: React.FC<{ resolution?: Resolution }> = ({ resolution }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    if (!resolution) return null;
    const tone = tierTone(resolution.outcome.tier, c);
    const terms = resolution.terms ?? [];

    return (
        <Card theme={theme} surface="surface" radius="md" bordered style={[styles.card, { borderColor: c.hairline }]}>
            <View style={styles.termsRow}>
                {terms.map((t, i) => (
                    <Text key={i} theme={theme} variant="mono" style={[styles.term, { color: c.inkMuted }]}>
                        {t.label} {t.value >= 0 ? `+${t.value}` : t.value}
                    </Text>
                ))}
                <Text theme={theme} variant="mono" style={[styles.term, { color: c.inkFaint }]}>
                    ПРОТИВ {resolution.target}
                </Text>
            </View>
            <View style={[styles.outcome, { backgroundColor: tone.fill }]}>
                <Text theme={theme} variant="mono" style={[styles.outcomeLabel, { color: tone.fg }]}>
                    {resolution.outcome.label.toUpperCase()}
                </Text>
                <Text theme={theme} variant="mono" style={[styles.margin, { color: c.inkFaint }]}>
                    маржа {resolution.margin >= 0 ? `+${resolution.margin}` : resolution.margin}
                </Text>
            </View>
        </Card>
    );
};

const styles = StyleSheet.create({
    card: { gap: 8 },
    termsRow: { flexDirection: 'row', flexWrap: 'wrap', gap: 8, alignItems: 'baseline' },
    term: { fontSize: 10.5, letterSpacing: 0.4 },
    outcome: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        borderRadius: 8,
        paddingVertical: 6,
        paddingHorizontal: 10,
    },
    outcomeLabel: { fontSize: 11, letterSpacing: 1 },
    margin: { fontSize: 10 },
});
