import React from 'react';
import { ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Text } from '@genie/ds';
import { useTheme } from '../theme/ThemeContext';
import { Panel, OptionsRail } from '../components/turnview';
import { TurnView } from '../turnview/types';
import { Intent } from '../turnview/intents';

// Досье (8a, обзор) — рабочее место дедукции: обобщённая Panel (секции + слоты) от сценария.
// Сборка обвинения (8b) — вне этого захода; переключатель показан, «обвинение» неактивно.
// Анти-спойлер держит сам компонент Panel (ничего «верным/зелёным»).
export const DossierScreen: React.FC<{ view: TurnView; onIntent: (i: Intent) => void }> = ({ view, onIntent }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const insets = useSafeAreaInsets();

    return (
        <View style={[styles.root, { backgroundColor: c.background }]}>
            <ScrollView
                contentContainerStyle={[styles.content, { paddingTop: insets.top + 12, paddingBottom: insets.bottom + 20 }]}
                showsVerticalScrollIndicator={false}
            >
                <View style={[styles.modeSwitch, { borderColor: c.stroke ?? c.border }]}>
                    <View style={[styles.mode, styles.modeActive, { backgroundColor: c.amberFill }]}>
                        <Text theme={theme} variant="mono" style={[styles.modeText, { color: c.amberText }]}>
                            ОБЗОР
                        </Text>
                    </View>
                    <View style={styles.mode}>
                        <Text theme={theme} variant="mono" style={[styles.modeText, { color: c.inkFaint }]}>
                            ОБВИНЕНИЕ
                        </Text>
                    </View>
                </View>

                <Panel panel={view.objective} onIntent={onIntent} />

                <OptionsRail options={view.options} onIntent={onIntent} layout="list" />
            </ScrollView>
        </View>
    );
};

const styles = StyleSheet.create({
    root: { flex: 1 },
    content: { paddingHorizontal: 18, gap: 16 },
    modeSwitch: { flexDirection: 'row', borderWidth: 1, borderRadius: 999, padding: 3, alignSelf: 'flex-start' },
    mode: { paddingVertical: 6, paddingHorizontal: 14, borderRadius: 999 },
    modeActive: {},
    modeText: { fontSize: 9.5, letterSpacing: 1 },
});
