import React from 'react';
import { Pressable, StyleSheet, View } from 'react-native';
import { LinearGradient } from 'expo-linear-gradient';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Text } from '@genie/ds';
import { useTheme } from '../theme/ThemeContext';
import { Icon, IconName } from '../components/Icon';
import { TabKey } from '../state/store';

// Навигационный док (12a) — плавающая пилюля, 5 табов, приподнятый центральный слот «Дело»
// (амбер-круг) с бейджем «ход», точка события у «Мир», DockFade-градиент, safe-area.
// Это оболочка, НЕ командная панель сессии.

type Item = { key: TabKey; label: string; icon: IconName; center?: boolean; badge?: 'turn' | 'dot' };

const ITEMS: Item[] = [
    { key: 'home', label: 'Дом', icon: 'home' },
    { key: 'world', label: 'Мир', icon: 'globe', badge: 'dot' },
    { key: 'case', label: 'Дело', icon: 'magnifier', center: true, badge: 'turn' },
    { key: 'hero', label: 'Герой', icon: 'user' },
    { key: 'more', label: 'Ещё', icon: 'dots' },
];

export const FloatingDock: React.FC<{ active: TabKey; onSelect: (t: TabKey) => void }> = ({ active, onSelect }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();

    return (
        <View pointerEvents="box-none" style={[styles.wrap, { paddingBottom: Math.max(insets.bottom, 10) }]}>
            <LinearGradient
                colors={['transparent', 'rgba(15,13,10,0.72)', c.background]}
                locations={[0, 0.55, 1]}
                style={styles.fade}
                pointerEvents="none"
            />
            <View style={[styles.dock, { backgroundColor: c.surfaceAlt, borderColor: 'rgba(196,122,44,0.4)' }]}>
                {ITEMS.map((it) => (
                    <DockTab key={it.key} item={it} active={active === it.key} onPress={() => onSelect(it.key)} />
                ))}
            </View>
        </View>
    );
};

const DockTab: React.FC<{ item: Item; active: boolean; onPress: () => void }> = ({ item, active, onPress }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;

    if (item.center) {
        return (
            <Pressable onPress={onPress} hitSlop={8} style={styles.centerSlot}>
                <View style={[styles.centerCircle, { backgroundColor: c.accent, borderColor: c.surfaceAlt }]}>
                    <Icon name={item.icon} size={22} color={c.background} />
                    {item.badge === 'turn' ? (
                        <View style={[styles.turnBadge, { backgroundColor: c.blood ?? c.error }]}>
                            <Text theme={theme} variant="mono" style={styles.turnText}>
                                ХОД
                            </Text>
                        </View>
                    ) : null}
                </View>
                <Text theme={theme} variant="mono" style={[styles.label, { color: active ? c.amberText : c.inkFaint }]}>
                    {item.label.toUpperCase()}
                </Text>
            </Pressable>
        );
    }

    return (
        <Pressable onPress={onPress} hitSlop={8} style={styles.tab}>
            <View style={styles.iconWrap}>
                {active ? <View style={[styles.activePill, { backgroundColor: c.amberFill }]} /> : null}
                <Icon name={item.icon} size={21} color={active ? c.amberText ?? c.accent : c.inkMuted ?? c.textSecondary} />
                {item.badge === 'dot' ? <View style={[styles.dot, { backgroundColor: c.accent }]} /> : null}
            </View>
            <Text theme={theme} variant="mono" style={[styles.label, { color: active ? c.amberText : c.inkFaint }]}>
                {item.label.toUpperCase()}
            </Text>
        </Pressable>
    );
};

const styles = StyleSheet.create({
    wrap: { position: 'absolute', left: 0, right: 0, bottom: 0, alignItems: 'center' },
    fade: { position: 'absolute', left: 0, right: 0, bottom: 0, height: 120 },
    dock: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        borderWidth: 1,
        borderRadius: 22,
        paddingHorizontal: 12,
        paddingVertical: 10,
        marginHorizontal: 16,
        width: '92%',
        shadowColor: '#000',
        shadowOffset: { width: 0, height: 12 },
        shadowOpacity: 0.8,
        shadowRadius: 18,
        elevation: 10,
    },
    tab: { alignItems: 'center', gap: 4, flex: 1 },
    iconWrap: { alignItems: 'center', justifyContent: 'center', width: 44, height: 30 },
    activePill: { position: 'absolute', width: 44, height: 26, borderRadius: 999 },
    dot: { position: 'absolute', top: 2, right: 8, width: 6, height: 6, borderRadius: 999 },
    centerSlot: { alignItems: 'center', gap: 4, flex: 1 },
    centerCircle: {
        width: 52,
        height: 52,
        borderRadius: 26,
        borderWidth: 3,
        alignItems: 'center',
        justifyContent: 'center',
        marginTop: -24,
        shadowColor: 'rgba(196,122,44,0.6)',
        shadowOffset: { width: 0, height: 8 },
        shadowOpacity: 0.6,
        shadowRadius: 14,
        elevation: 8,
    },
    turnBadge: { position: 'absolute', top: -6, right: -10, borderRadius: 999, paddingHorizontal: 6, paddingVertical: 2 },
    turnText: { fontSize: 7.5, letterSpacing: 0.8, color: '#F5EFDD' },
    label: { fontSize: 8.5, letterSpacing: 0.8 },
});
