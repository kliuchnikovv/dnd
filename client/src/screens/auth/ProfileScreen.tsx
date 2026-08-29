import React from 'react';
import { StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Button, Card, Text } from '@genie/ds';
import { useTheme } from '../../theme/ThemeContext';
import { useAuth } from '../../state/useAuth';

// ProfileScreen — минимальный экран профиля: кто вошёл + выход. Логика — в
// useAuth().logout(); экран только показывает user и зовёт действие.
export const ProfileScreen: React.FC = () => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    const { user, logout } = useAuth();

    return (
        <View style={[styles.root, { backgroundColor: c.background, paddingTop: insets.top + 24, paddingBottom: insets.bottom + 24 }]}>
            <Card theme={theme} surface="surface" radius="lg" bordered style={styles.card}>
                <Text theme={theme} variant="title" style={{ color: c.text }}>
                    {user?.name ?? '—'}
                </Text>
                <Text theme={theme} variant="body" tone="secondary" style={styles.email}>
                    {user?.email ?? '—'}
                </Text>
            </Card>
            <Button theme={theme} label="Выйти" variant="secondary" size="md" onPress={() => logout()} />
        </View>
    );
};

const styles = StyleSheet.create({
    root: {
        flex: 1,
        justifyContent: 'space-between',
        paddingHorizontal: 24,
    },
    card: {
        gap: 4,
    },
    email: {
        marginTop: 4,
    },
});
