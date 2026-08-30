import React, { useRef } from 'react';
import { KeyboardAvoidingView, Platform, ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Text } from '@genie/ds';
import { useTheme } from '../theme/ThemeContext';
import { MeterRow, NarrationFeed, OptionsRail, Composer } from '../components/turnview';
import { TurnView } from '../turnview/types';
import { Intent } from '../turnview/intents';
import { havenMeterTone } from '../mocks/haven/skin';

// Сцена (6b) — главный игровой цикл: место + лента повествования + действия + свободный ввод.
// Хедер (место + меры) липкий — вне ScrollView; композер уводится клавиатурой через
// KeyboardAvoidingView. Верхний отступ хедера оставляет место под пилюлю «выйти» (AppShell).
export const SceneScreen: React.FC<{ view: TurnView; onIntent: (i: Intent) => void }> = ({ view, onIntent }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    const scrollRef = useRef<ScrollView>(null);

    return (
        <KeyboardAvoidingView
            style={[styles.root, { backgroundColor: c.background }]}
            behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        >
            <View style={[styles.header, { paddingTop: insets.top + 48, borderBottomColor: c.hairline, backgroundColor: c.background }]}>
                <Text theme={theme} variant="title" style={{ color: c.text }}>
                    {view.scene.title}
                </Text>
                <MeterRow meters={view.meters} tone={havenMeterTone} />
            </View>

            <ScrollView
                ref={scrollRef}
                style={styles.feed}
                contentContainerStyle={styles.content}
                showsVerticalScrollIndicator={false}
                onContentSizeChange={() => scrollRef.current?.scrollToEnd({ animated: true })}
            >
                <NarrationFeed narration={view.narration} />
            </ScrollView>

            <View style={[styles.dock, { backgroundColor: c.background, borderTopColor: c.hairline, paddingBottom: insets.bottom + 8 }]}>
                <OptionsRail options={view.options} onIntent={onIntent} layout="rail" />
                <Composer onIntent={onIntent} />
            </View>
        </KeyboardAvoidingView>
    );
};

const styles = StyleSheet.create({
    root: { flex: 1 },
    header: { paddingHorizontal: 18, paddingBottom: 12, gap: 10, borderBottomWidth: 1 },
    feed: { flex: 1 },
    content: { paddingHorizontal: 18, paddingTop: 14, paddingBottom: 20, gap: 14 },
    dock: { paddingHorizontal: 18, paddingTop: 10, borderTopWidth: 1, gap: 10 },
});
