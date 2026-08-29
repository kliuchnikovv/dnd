import React from 'react';
import { StyleSheet, View } from 'react-native';

import { Text } from '@genie/ds';
import { ThemeColors, ThemeDescriptor } from '@genie/front/themes';
import { useTheme } from '../../theme/ThemeContext';
import { Meter } from '../../turnview/types';

// MeterRow — меры от ПРАВИЛА. Агностичен: рисует из label/value/max, по kind НЕ ветвится
// (зеркало cli/render.go surfacedMeters). Показывает ТОЛЬКО surface:true. Окраска — опциональный
// скин конфиг-слоя (mocks/haven/skin.ts); без него всё красится акцентом.

export type MeterTone = (m: Meter, c: ThemeColors) => string;

const MAX_PIPS = 8;

export const MeterRow: React.FC<{ meters?: Meter[]; tone?: MeterTone }> = ({ meters, tone }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const surfaced = (meters ?? []).filter((m) => m.surface);
    if (surfaced.length === 0) return null;

    return (
        <View style={styles.row}>
            {surfaced.map((m, i) => {
                const color = tone ? tone(m, c) : c.accent;
                return (
                    <View key={`${m.label}-${i}`} style={styles.meter}>
                        <Text theme={theme} variant="mono" style={[styles.label, { color: c.inkMuted }]}>
                            {m.label.toUpperCase()}
                        </Text>
                        {renderGauge(m, color, theme)}
                    </View>
                );
            })}
        </View>
    );
};

function renderGauge(m: Meter, color: string, theme: ThemeDescriptor) {
    const c = theme.colors;
    // С потолком и в разумных пределах — сегментные пипы. Иначе — тонкий бар. Без потолка — число.
    if (m.max !== undefined && m.max > 0 && m.max <= MAX_PIPS) {
        return (
            <View style={styles.pips}>
                {Array.from({ length: m.max }).map((_, i) => (
                    <View
                        key={i}
                        style={[
                            styles.pip,
                            { borderColor: color, backgroundColor: i < m.value ? color : 'transparent' },
                        ]}
                    />
                ))}
            </View>
        );
    }
    if (m.max !== undefined && m.max > 0) {
        const ratio = Math.max(0, Math.min(1, m.value / m.max));
        return (
            <View style={[styles.barTrack, { backgroundColor: c.stroke ?? c.border }]}>
                <View style={[styles.barFill, { width: `${ratio * 100}%`, backgroundColor: color }]} />
            </View>
        );
    }
    return (
        <Text theme={theme} variant="mono" style={{ color }}>
            {String(m.value)}
        </Text>
    );
}

const styles = StyleSheet.create({
    row: { flexDirection: 'row', flexWrap: 'wrap', gap: 14, alignItems: 'center' },
    meter: { flexDirection: 'row', alignItems: 'center', gap: 6 },
    label: { letterSpacing: 1, fontSize: 9.5 },
    pips: { flexDirection: 'row', gap: 3 },
    pip: { width: 7, height: 7, borderRadius: 999, borderWidth: 1.4 },
    barTrack: { width: 48, height: 4, borderRadius: 999, overflow: 'hidden' },
    barFill: { height: 4, borderRadius: 999 },
});
