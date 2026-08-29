import React from 'react';
import { View } from 'react-native';

import { Button, Card, Chip, Text } from '@genie/ds';
import { useTheme } from '../theme/ThemeContext';

// Dev-only smoke check: proves vendored @genie/ds + «Ночной детектив» theme + the three
// font voices all render. Replaced by real screens in later phases.
export const ReferenceCard: React.FC = () => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;

    return (
        <Card theme={theme} surface="surface" radius="lg" bordered elevation="raised" style={{ width: '100%', gap: 12 }}>
            <Text theme={theme} variant="display">Дознание</Text>
            <Text theme={theme} variant="mono" style={{ color: c.inkMuted, textTransform: 'uppercase' }}>
                ночной детектив · каркас
            </Text>
            <Text theme={theme} variant="body" style={{ color: c.inkSecondary, fontStyle: 'italic' }}>
                Фонарь качается над водой. Пристань молчит, но что-то в этой тишине не сходится.
            </Text>
            <View style={{ flexDirection: 'row', gap: 8, flexWrap: 'wrap' }}>
                <Chip theme={theme} label="чутьё" tone="accent" variant="soft" />
                <Chip theme={theme} label="факт ✓" tone="success" variant="soft" />
                <Chip theme={theme} label="отказ" tone="error" variant="soft" />
            </View>
            <Button theme={theme} label="сделать ход" variant="primary" fullWidth />
        </Card>
    );
};
