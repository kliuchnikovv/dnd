import React, { useEffect, useRef } from 'react';
import { Animated, Pressable, ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Button, Card, Text } from '@genie/ds';
import { useTheme } from '../theme/ThemeContext';
import { Icon } from '../components/Icon';
import { Avatar } from '../components/turnview';
import { havenHub, HubModel } from '../mocks/app';
import { useAppStore } from '../state/store';

// Хаб (13b) — точка входа: async-каденс на поверхности. Пульсирующая приоритетная карточка
// «твой ход готов» + дайджест «мир сдвинулся» + мини-карточка героя.
export const HubScreen: React.FC<{ model?: HubModel }> = ({ model = havenHub }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    const setTab = useAppStore((s) => s.setTab);

    const roleColor = (role: 'wound' | 'moss' | 'faint') =>
        role === 'wound' ? c.wound : role === 'moss' ? c.moss : c.inkFaint;

    return (
        <View style={[styles.root, { backgroundColor: c.background }]}>
            <ScrollView
                contentContainerStyle={[styles.content, { paddingTop: insets.top + 16, paddingBottom: insets.bottom + 120 }]}
                showsVerticalScrollIndicator={false}
            >
                <Text theme={theme} variant="display" style={{ color: c.text }}>
                    Дознание
                </Text>

                <PulsePriorityCard model={model} onGo={() => setTab('case')} />

                <Card theme={theme} surface="surface" radius="lg" bordered style={[styles.digest, { borderColor: c.hairline }]}>
                    <View style={styles.digestHead}>
                        <Icon name="globe" size={16} color={c.inkMuted ?? c.textSecondary} />
                        <Text theme={theme} variant="mono" style={[styles.digestTitle, { color: c.inkMuted }]}>
                            МИР СДВИНУЛСЯ, ПОКА ТЕБЯ НЕ БЫЛО
                        </Text>
                    </View>
                    {model.worldDigest.map((e) => (
                        <View key={e.id} style={styles.event}>
                            <View style={[styles.eventDot, { backgroundColor: roleColor(e.role) }]} />
                            <Text theme={theme} variant="body" style={[styles.eventText, { color: c.inkSecondary }]}>
                                {e.text}
                            </Text>
                        </View>
                    ))}
                </Card>

                <Card theme={theme} surface="surface" radius="lg" style={styles.hero}>
                    <Avatar who={{ id: 'pc', name: model.heroName, disposition: 0 }} size={40} />
                    <View style={styles.heroText}>
                        <Text theme={theme} variant="title" style={{ color: c.text }}>
                            {model.heroName}
                        </Text>
                        <Text theme={theme} variant="mono" style={[styles.heroRep, { color: c.inkMuted }]}>
                            {model.heroReputation}
                        </Text>
                    </View>
                </Card>
            </ScrollView>
        </View>
    );
};

const PulsePriorityCard: React.FC<{ model: HubModel; onGo: () => void }> = ({ model, onGo }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const pulse = useRef(new Animated.Value(0)).current;

    useEffect(() => {
        const loop = Animated.loop(
            Animated.sequence([
                Animated.timing(pulse, { toValue: 1, duration: 1100, useNativeDriver: false }),
                Animated.timing(pulse, { toValue: 0, duration: 1100, useNativeDriver: false }),
            ]),
        );
        loop.start();
        return () => loop.stop();
    }, [pulse]);

    const borderColor = pulse.interpolate({ inputRange: [0, 1], outputRange: ['rgba(196,122,44,0.4)', 'rgba(196,122,44,1)'] });

    return (
        <Animated.View style={[styles.priority, { backgroundColor: c.surfaceHighlight, borderColor }]}>
            <Text theme={theme} variant="mono" style={[styles.priorityKicker, { color: c.amberText }]}>
                ТВОЙ ХОД ГОТОВ
            </Text>
            <Text theme={theme} variant="title" style={{ color: c.text }}>
                {model.priority.caseTitle}
            </Text>
            <Text theme={theme} variant="body" style={[styles.priorityLine, { color: c.inkSecondary }]}>
                {model.priority.line}
            </Text>
            <Button theme={theme} label={model.priority.cta} variant="primary" onPress={onGo} style={styles.cta} />
        </Animated.View>
    );
};

const styles = StyleSheet.create({
    root: { flex: 1 },
    content: { paddingHorizontal: 18, gap: 16 },
    priority: { borderWidth: 1.5, borderRadius: 16, padding: 16, gap: 8 },
    priorityKicker: { fontSize: 9.5, letterSpacing: 1.4 },
    priorityLine: { fontStyle: 'italic' },
    cta: { marginTop: 4 },
    digest: { gap: 10 },
    digestHead: { flexDirection: 'row', alignItems: 'center', gap: 6 },
    digestTitle: { fontSize: 9, letterSpacing: 1.2 },
    event: { flexDirection: 'row', alignItems: 'flex-start', gap: 8 },
    eventDot: { width: 6, height: 6, borderRadius: 999, marginTop: 8 },
    eventText: { flexShrink: 1 },
    hero: { flexDirection: 'row', alignItems: 'center', gap: 12 },
    heroText: { gap: 2 },
    heroRep: { fontSize: 10, letterSpacing: 0.4 },
});
