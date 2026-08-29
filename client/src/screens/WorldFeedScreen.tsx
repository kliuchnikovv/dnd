import React from 'react';
import { FlatList, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Button, Card, Text } from '@genie/ds';
import { useTheme } from '../theme/ThemeContext';
import { Icon, IconName } from '../components/Icon';
import { havenWorldFeed, WorldEvent } from '../mocks/app';
import { useAppStore } from '../state/store';

// Лента Мира (15a, регион-скоуп) — хроника последствий, НЕ соцсеть. Две карточки: ambient
// (плоская, read-only) и хук (surface-raised + амбер-рамка + действие → интент/дело).
// Барьер: только структурный канон, никакого прямого контента игроков.
export const WorldFeedScreen: React.FC<{ events?: WorldEvent[] }> = ({ events = havenWorldFeed }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    const setTab = useAppStore((s) => s.setTab);

    return (
        <View style={[styles.root, { backgroundColor: c.background }]}>
            <FlatList
                data={events}
                keyExtractor={(e) => e.id}
                contentContainerStyle={[styles.content, { paddingTop: insets.top + 16, paddingBottom: insets.bottom + 120 }]}
                showsVerticalScrollIndicator={false}
                ListHeaderComponent={
                    <Text theme={theme} variant="display" style={[styles.title, { color: c.text }]}>
                        Мир
                    </Text>
                }
                renderItem={({ item }) => <EventCard event={item} onHook={() => setTab('case')} />}
            />
        </View>
    );
};

const EventCard: React.FC<{ event: WorldEvent; onHook: () => void }> = ({ event, onHook }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const isHook = event.kind === 'hook';

    return (
        <Card
            theme={theme}
            surface={isHook ? 'surfaceHighlight' : 'surface'}
            radius="lg"
            bordered
            style={[styles.card, { borderColor: isHook ? c.accent : c.hairline }]}
        >
            <View style={styles.head}>
                <Icon name={event.icon as IconName} size={16} color={isHook ? c.amberText ?? c.accent : c.inkMuted ?? c.textSecondary} />
                {isHook ? (
                    <Text theme={theme} variant="mono" style={[styles.kicker, { color: c.amberText }]}>
                        ХУК
                    </Text>
                ) : null}
                <Text theme={theme} variant="mono" style={[styles.meta, { color: c.inkFaint }]}>
                    {event.meta}
                </Text>
            </View>
            <Text theme={theme} variant="body" style={[styles.body, { color: c.inkSecondary }]}>
                {event.text}
            </Text>
            {isHook && event.action ? (
                <Button theme={theme} label={event.action.label} variant="primary" size="sm" onPress={onHook} style={styles.action} />
            ) : null}
        </Card>
    );
};

const styles = StyleSheet.create({
    root: { flex: 1 },
    content: { paddingHorizontal: 18, gap: 12 },
    title: { marginBottom: 4 },
    card: { gap: 8 },
    head: { flexDirection: 'row', alignItems: 'center', gap: 6 },
    kicker: { fontSize: 9, letterSpacing: 1.2 },
    meta: { fontSize: 9.5, letterSpacing: 0.6, marginLeft: 'auto' },
    body: { fontStyle: 'italic' },
    action: { marginTop: 2 },
});
