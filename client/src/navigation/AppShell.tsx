import React from 'react';
import { Pressable, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Text } from '@genie/ds';
import { useTheme } from '../theme/ThemeContext';
import { Icon } from '../components/Icon';
import { FloatingDock } from './FloatingDock';
import { useAppStore } from '../state/store';
import { HubScreen } from '../screens/HubScreen';
import { WorldFeedScreen } from '../screens/WorldFeedScreen';
import { SessionScreen } from '../screens/SessionScreen';
import { CharacterCreateScreen } from '../screens/CharacterCreateScreen';

// AppShell — оболочка: FloatingDock (12a) + маршрутизация табов. В сессии («Дело») док уступает
// место (immersive), а выход даёт мини-хэндл сверху — так композер сессии не спорит с доком.
export const AppShell: React.FC = () => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    const tab = useAppStore((s) => s.tab);
    const setTab = useAppStore((s) => s.setTab);

    const inSession = tab === 'case';

    return (
        <View style={[styles.root, { backgroundColor: c.background }]}>
            {tab === 'home' ? <HubScreen /> : null}
            {tab === 'world' ? <WorldFeedScreen /> : null}
            {tab === 'case' ? <SessionScreen /> : null}
            {tab === 'hero' ? <CharacterCreateScreen /> : null}
            {tab === 'more' ? <MoreScreen /> : null}

            {inSession ? (
                <Pressable
                    onPress={() => setTab('home')}
                    hitSlop={10}
                    style={[styles.exit, { top: insets.top + 8, backgroundColor: c.surfaceAlt, borderColor: c.stroke ?? c.border }]}
                >
                    <Icon name="back" size={16} color={c.inkMuted ?? c.textSecondary} />
                    <Text theme={theme} variant="mono" style={[styles.exitText, { color: c.inkMuted }]}>
                        ВЫЙТИ
                    </Text>
                </Pressable>
            ) : (
                <FloatingDock active={tab} onSelect={setTab} />
            )}
        </View>
    );
};

const MoreScreen: React.FC = () => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    return (
        <View style={[styles.more, { backgroundColor: c.background, paddingTop: insets.top + 24 }]}>
            <Text theme={theme} variant="display" style={{ color: c.text }}>
                Ещё
            </Text>
            <Text theme={theme} variant="body" style={{ color: c.inkMuted }}>
                Настройки, профиль, тема, подписка — рутина следующего захода.
            </Text>
        </View>
    );
};

const styles = StyleSheet.create({
    root: { flex: 1 },
    exit: {
        position: 'absolute',
        right: 16,
        flexDirection: 'row',
        alignItems: 'center',
        gap: 4,
        borderWidth: 1,
        borderRadius: 999,
        paddingVertical: 6,
        paddingHorizontal: 12,
    },
    exitText: { fontSize: 9, letterSpacing: 1 },
    more: { flex: 1, paddingHorizontal: 18, gap: 12 },
});
