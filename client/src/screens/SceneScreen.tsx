import React from 'react';
import { ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Card, Text } from '@genie/ds';
import { useTheme } from '../theme/ThemeContext';
import { MeterRow, NarrationFeed, OptionsRail, ResolutionCard, Composer } from '../components/turnview';
import { TurnView } from '../turnview/types';
import { Intent } from '../turnview/intents';
import { havenMeterTone } from '../mocks/haven/skin';

// Сцена (6b) — главный игровой цикл: место + лента повествования + действия + свободный ввод.
export const SceneScreen: React.FC<{ view: TurnView; onIntent: (i: Intent) => void }> = ({ view, onIntent }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();

    return (
        <View style={[styles.root, { backgroundColor: c.background }]}>
            <ScrollView
                contentContainerStyle={[styles.content, { paddingTop: insets.top + 12, paddingBottom: 20 }]}
                showsVerticalScrollIndicator={false}
            >
                <Card theme={theme} surface="surface" radius="lg" bordered style={[styles.place, { borderColor: c.hairline }]}>
                    <Text theme={theme} variant="title" style={{ color: c.text }}>
                        {view.scene.title}
                    </Text>
                    <MeterRow meters={view.meters} tone={havenMeterTone} />
                </Card>

                <ResolutionCard resolution={view.resolution} />
                <NarrationFeed narration={view.narration} />
            </ScrollView>

            <View style={[styles.dock, { backgroundColor: c.background, borderTopColor: c.hairline, paddingBottom: insets.bottom + 8 }]}>
                <OptionsRail options={view.options} onIntent={onIntent} layout="rail" />
                <Composer onIntent={onIntent} />
            </View>
        </View>
    );
};

const styles = StyleSheet.create({
    root: { flex: 1 },
    content: { paddingHorizontal: 18, gap: 14 },
    place: { gap: 10 },
    dock: { paddingHorizontal: 18, paddingTop: 10, borderTopWidth: 1, gap: 10 },
});
