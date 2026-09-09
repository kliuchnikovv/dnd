import { useEffect, useState } from 'react';
import { StatusBar } from 'expo-status-bar';
import { ActivityIndicator, StyleSheet, View } from 'react-native';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { useStore } from 'zustand';

import { ThemeProvider } from './src/theme/ThemeContext';
import { useAppFonts } from './src/theme/fonts';
import { nightDetectiveTheme } from './src/theme/nightDetective';
import { authStore } from './src/state/useAuth';
import { screenFor } from './src/state/gate';
import { SignInScreen } from './src/screens/auth/SignInScreen';
import { SessionSelectScreen, SessionPick } from './src/screens/session/SessionSelectScreen';
import { SessionScreen } from './src/screens/session/SessionScreen';
import { VignetteScreen } from './src/components/vignette/VignetteScreen';

export default function App() {
    const fontsLoaded = useAppFonts();
    const status = useStore(authStore, (s) => s.status);
    // pick — какая сессия открыта и по какому виду её вести (turn|vignette,
    // см. screens/session/sessionKind.ts). Держим локально: гейт signedIn →
    // выбор сессии → сама игра не завязан на постоянное состояние, при выходе
    // (signedOut) сбрасывается автоматически ниже.
    const [pick, setPick] = useState<SessionPick | null>(null);

    useEffect(() => {
        authStore.getState().restore();
    }, []);

    useEffect(() => {
        if (status !== 'signedIn') setPick(null);
    }, [status]);

    const screen = screenFor(status);

    return (
        <GestureHandlerRootView style={{ flex: 1 }}>
            <SafeAreaProvider>
                <ThemeProvider>
                    <View style={[styles.root, { backgroundColor: nightDetectiveTheme.colors.background }]}>
                        {fontsLoaded && screen === 'app' ? (
                            pick ? (
                                pick.kind === 'vignette' ? (
                                    <VignetteScreen chatId={pick.chatId} />
                                ) : (
                                    <SessionScreen chatId={pick.chatId} />
                                )
                            ) : (
                                <SessionSelectScreen onPick={setPick} />
                            )
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
