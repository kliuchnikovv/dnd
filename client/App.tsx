import { useEffect } from 'react';
import { StatusBar } from 'expo-status-bar';
import { ActivityIndicator, StyleSheet, View } from 'react-native';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { useStore } from 'zustand';

import { ThemeProvider } from './src/theme/ThemeContext';
import { useAppFonts } from './src/theme/fonts';
import { nightDetectiveTheme } from './src/theme/nightDetective';
import { AppShell } from './src/navigation/AppShell';
import { authStore } from './src/state/useAuth';
import { screenFor } from './src/state/gate';
import { SignInScreen } from './src/screens/auth/SignInScreen';

export default function App() {
    const fontsLoaded = useAppFonts();
    const status = useStore(authStore, (s) => s.status);

    useEffect(() => {
        authStore.getState().restore();
    }, []);

    const screen = screenFor(status);

    return (
        <GestureHandlerRootView style={{ flex: 1 }}>
            <SafeAreaProvider>
                <ThemeProvider>
                    <View style={[styles.root, { backgroundColor: nightDetectiveTheme.colors.background }]}>
                        {fontsLoaded && screen === 'app' ? (
                            <AppShell />
                        ) : fontsLoaded && screen === 'signin' ? (
                            <SignInScreen />
                        ) : (
                            <View style={styles.center}>
                                <ActivityIndicator color={nightDetectiveTheme.colors.accent} />
                            </View>
                        )}
                        <StatusBar style="light" />
                    </View>
                </ThemeProvider>
            </SafeAreaProvider>
        </GestureHandlerRootView>
    );
}

const styles = StyleSheet.create({
    root: {
        flex: 1,
    },
    center: {
        flex: 1,
        alignItems: 'center',
        justifyContent: 'center',
    },
});
