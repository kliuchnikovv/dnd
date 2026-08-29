import React, { useMemo } from 'react';
import { ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Text } from '@genie/ds';
import { useTheme } from '../theme/ThemeContext';
import {
    Composer,
    MapGraph,
    MeterRow,
    NarrationFeed,
    OptionsRail,
    Panel,
    Participants,
    ResolutionCard,
} from '../components/turnview';
import { useTurnView } from '../state/useTurnView';
import { createHavenSource } from '../mocks/haven/case';
import { havenMeterTone } from '../mocks/haven/skin';

// Демо Фазы 3: весь обобщённый рендерер против фикстур «Гавань», с живым плумбингом интентов.
// Не финальный экран — доказательство, что дескрипторы рисуются и токены доходят до источника.
export const RendererDemo: React.FC = () => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    const source = useMemo(() => createHavenSource(), []);
    const { view, send } = useTurnView(source);

    return (
        <View style={[styles.root, { backgroundColor: c.background }]}>
            <ScrollView
                contentContainerStyle={[styles.content, { paddingTop: insets.top + 12, paddingBottom: 24 }]}
                showsVerticalScrollIndicator={false}
            >
                <View style={styles.header}>
                    <Text theme={theme} variant="display" style={{ color: c.text }}>
                        {view.scene.title}
                    </Text>
                    <MeterRow meters={view.meters} tone={havenMeterTone} />
                </View>

                <Participants participants={view.participants} />
                <ResolutionCard resolution={view.resolution} />
                <NarrationFeed narration={view.narration} />

                {view.map ? (
                    <MapGraph
                        map={view.map}
                        currentNodeId="harbor.tavern"
                        onIntent={send}
                        moveTokenFor={(n) =>
                            view.options?.find((o) => o.label.toLowerCase().includes(n.name.toLowerCase()))?.token
                        }
                    />
                ) : null}

                <Panel panel={view.objective} onIntent={send} />

                <View style={styles.actions}>
                    <OptionsRail options={view.options} onIntent={send} layout="list" />
                </View>
            </ScrollView>

            <View style={[styles.composer, { paddingBottom: insets.bottom + 8, borderTopColor: c.hairline, backgroundColor: c.background }]}>
                <Composer onIntent={send} />
            </View>
        </View>
    );
};

const styles = StyleSheet.create({
    root: { flex: 1 },
    content: { paddingHorizontal: 18, gap: 16 },
    header: { gap: 10 },
    actions: { gap: 8 },
    composer: { paddingHorizontal: 18, paddingTop: 8, borderTopWidth: 1 },
});
