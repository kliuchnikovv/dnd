import { StatusBar } from 'expo-status-bar';
import { ActivityIndicator, StyleSheet, View } from 'react-native';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { ThemeProvider } from './src/theme/ThemeContext';
import { useAppFonts } from './src/theme/fonts';
import { nightDetectiveTheme } from './src/theme/nightDetective';
import { AppShell } from './src/navigation/AppShell';

export default function App() {
    const fontsLoaded = useAppFonts();

    return (
        <GestureHandlerRootView style={{ flex: 1 }}>
            <SafeAreaProvider>
                <ThemeProvider>
                    <View style={[styles.root, { backgroundColor: nightDetectiveTheme.colors.background }]}>
                        {fontsLoaded ? (
                            <AppShell />
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
