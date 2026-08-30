import React from 'react';
import { StyleSheet, View } from 'react-native';

import { Text } from '@genie/ds';
import { useTheme } from '../../theme/ThemeContext';
import { Who } from '../../turnview/types';
import { dispositionTone } from './tone';
import { listKey } from './keys';

// Participants — присутствующие. Аватар пока плейсхолдер-монограмма (арт — асинхронно из
// асс-стора, immutable-по-ссылке). Кольцо аватара красится по знаку disposition (generic).

export const Avatar: React.FC<{ who: Who; size?: number }> = ({ who, size = 34 }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const ring = dispositionTone(who.disposition, c);
    const monogram = who.name.trim().slice(0, 1).toUpperCase();
    return (
        <View
            style={[
                styles.avatar,
                { width: size, height: size, borderRadius: size / 2, borderColor: ring, backgroundColor: c.surfaceHighlight },
            ]}
        >
            <Text theme={theme} variant="title" style={[styles.mono, { color: c.amberText, fontSize: size * 0.42 }]}>
                {monogram}
            </Text>
        </View>
    );
};

export const Participants: React.FC<{ participants?: Who[] }> = ({ participants }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    // Игрока в списке присутствующих не подписываем (правило контента): показываем NPC.
    const npcs = (participants ?? []).filter((w) => w.id !== 'pc');
    if (npcs.length === 0) return null;
    return (
        <View style={styles.row}>
            {npcs.map((w, i) => (
                <View key={listKey(w.id, i)} style={styles.chip}>
                    <Avatar who={w} size={28} />
                    <Text theme={theme} variant="mono" style={[styles.name, { color: c.inkMuted }]}>
                        {w.name}
                    </Text>
                </View>
            ))}
        </View>
    );
};

const styles = StyleSheet.create({
    row: { flexDirection: 'row', flexWrap: 'wrap', gap: 10, alignItems: 'center' },
    chip: { flexDirection: 'row', alignItems: 'center', gap: 6 },
    avatar: { alignItems: 'center', justifyContent: 'center', borderWidth: 1.5 },
    mono: {},
    name: { fontSize: 10, letterSpacing: 0.4 },
});
