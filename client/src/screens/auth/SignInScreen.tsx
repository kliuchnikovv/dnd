import React, { useCallback, useState } from 'react';
import { StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Button, Text } from '@genie/ds';
import { useTheme } from '../../theme/ThemeContext';
import { useGoogleSignIn } from '../../auth/googleSignIn';
import { useAuth } from '../../state/useAuth';

// SignInScreen — единственный вход в приложение для неавторизованного
// пользователя. Тонкий: вся логика входа — в useAuth().login(); экран лишь
// достаёт реальный GoogleSignIn из хука и передаёт его явно (см. useAuth.ts).
export const SignInScreen: React.FC = () => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();
    const google = useGoogleSignIn();
    const { login, error } = useAuth();
    const [pending, setPending] = useState(false);

    const onPress = useCallback(async () => {
        setPending(true);
        try {
            await login(google);
        } finally {
            setPending(false);
        }
    }, [login, google]);

    return (
        <View style={[styles.root, { backgroundColor: c.background, paddingTop: insets.top + 32, paddingBottom: insets.bottom + 32 }]}>
            <View style={styles.center}>
                <Text theme={theme} variant="display" style={{ color: c.text }}>
                    Дознание
                </Text>
                <Text theme={theme} variant="body" tone="secondary" style={styles.subtitle}>
                    Войдите, чтобы продолжить расследование
                </Text>
            </View>
            <View style={styles.footer}>
                {error ? (
                    <Text theme={theme} variant="caption" tone="error" style={styles.error}>
                        {error}
                    </Text>
                ) : null}
                <Button
                    theme={theme}
                    label="Войти через Google"
                    variant="primary"
                    size="lg"
                    fullWidth
                    loading={pending}
                    onPress={onPress}
                />
            </View>
        </View>
    );
};

const styles = StyleSheet.create({
    root: {
        flex: 1,
        justifyContent: 'space-between',
        paddingHorizontal: 24,
    },
    center: {
        flex: 1,
        alignItems: 'center',
        justifyContent: 'center',
        gap: 8,
    },
    subtitle: {
        textAlign: 'center',
    },
    footer: {
        gap: 12,
    },
    error: {
        textAlign: 'center',
    },
});
