import React from 'react';
import { ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Card, Text } from '@genie/ds';
import { useTheme } from '../theme/ThemeContext';
import { MapGraph, OptionsRail } from '../components/turnview';
import { TurnView, MapNode } from '../turnview/types';
import { Intent } from '../turnview/intents';

// Карта (9a, чистый граф) — где я, куда могу, цена перехода (сдвиг часов). Скин-иллюстрация (9b) —
// вне этого захода. currentNodeId для мока — таверна, откуда вышли.
export const MapScreen: React.FC<{
    view: TurnView;
    onIntent: (i: Intent) => void;
    currentNodeId?: string;
}> = ({ view, onIntent, currentNodeId = 'harbor.tavern' }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();

    // node → intent token перехода: сопоставляем по имени узла со словами варианта (мок-эвристика).
    const moveTokenFor = (n: MapNode): string | undefined =>
        (view.options ?? []).find((o) => o.label.toLowerCase().includes(n.name.toLowerCase()))?.token;

    return (
        <View style={[styles.root, { backgroundColor: c.background }]}>
            <ScrollView
                contentContainerStyle={[styles.content, { paddingTop: insets.top + 12, paddingBottom: insets.bottom + 20 }]}
                showsVerticalScrollIndicator={false}
            >
                <Text theme={theme} variant="title" style={{ color: c.text }}>
                    {view.scene.title}
                </Text>

                <MapGraph
                    map={view.map}
                    currentNodeId={currentNodeId}
                    onIntent={onIntent}
                    moveTokenFor={moveTokenFor}
                    height={340}
                />

                <View style={styles.legend}>
                    <LegendDot color={c.accent} label="достижимо" />
                    <LegendDot color={c.inkMuted ?? c.textSecondary} label="известно" dashed />
                    <LegendDot color={c.inkFaint ?? c.textSecondary} label="туман" />
                </View>

                <Card theme={theme} surface="surfaceHighlight" radius="lg" bordered style={[styles.confirm, { borderColor: c.stroke ?? c.border }]}>
                    <Text theme={theme} variant="mono" style={[styles.cost, { color: c.inkMuted }]}>
                        ПЕРЕХОД · +1 К ЧАСАМ ДАВЛЕНИЯ
                    </Text>
                    <OptionsRail options={view.options} onIntent={onIntent} layout="list" />
                </Card>
            </ScrollView>
        </View>
    );
};

const LegendDot: React.FC<{ color: string; label: string; dashed?: boolean }> = ({ color, label, dashed }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    return (
        <View style={styles.legendItem}>
            <View style={[styles.dot, { borderColor: color, borderStyle: dashed ? 'dashed' : 'solid' }]} />
            <Text theme={theme} variant="mono" style={[styles.legendText, { color: c.inkMuted }]}>
                {label.toUpperCase()}
            </Text>
        </View>
    );
};

const styles = StyleSheet.create({
    root: { flex: 1 },
    content: { paddingHorizontal: 18, gap: 14 },
    legend: { flexDirection: 'row', gap: 14, justifyContent: 'center' },
    legendItem: { flexDirection: 'row', alignItems: 'center', gap: 5 },
    dot: { width: 10, height: 10, borderRadius: 999, borderWidth: 1.4 },
    legendText: { fontSize: 8.5, letterSpacing: 0.8 },
    confirm: { gap: 10 },
    cost: { fontSize: 9.5, letterSpacing: 1 },
});
