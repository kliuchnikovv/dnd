import React, { useState } from 'react';
import { Pressable, StyleSheet, TextInput, View } from 'react-native';
import Svg, { Path } from 'react-native-svg';

import { useTheme } from '../../theme/ThemeContext';
import { Intent, freeIntent } from '../../turnview/intents';

// Composer — свободный ввод игрока: равноправная альтернатива тапу по варианту. Шлёт freeIntent;
// сервер разворачивает и валидирует. Квадратная амбер-кнопка отправки 42×42 (дизайн-хендофф).

export const Composer: React.FC<{ onIntent: (intent: Intent) => void; placeholder?: string }> = ({
    onIntent,
    placeholder = 'сказать или сделать…',
}) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const [text, setText] = useState('');

    const submit = () => {
        const t = text.trim();
        if (t.length === 0) return;
        onIntent(freeIntent(t));
        setText('');
    };

    return (
        <View style={styles.row}>
            <TextInput
                value={text}
                onChangeText={setText}
                placeholder={placeholder}
                placeholderTextColor={c.inkFaint}
                onSubmitEditing={submit}
                returnKeyType="send"
                style={[
                    styles.input,
                    { backgroundColor: c.surface, borderColor: c.stroke ?? c.border, color: c.text, fontFamily: 'PTSerif_400Regular' },
                ]}
            />
            <Pressable
                onPress={submit}
                hitSlop={6}
                style={({ pressed }) => [styles.send, { backgroundColor: c.accent, opacity: pressed ? 0.85 : 1 }]}
            >
                <Svg width={18} height={18} viewBox="0 0 24 24">
                    <Path
                        d="M4 12l16-8-6 16-3-6-7-2z"
                        fill="none"
                        stroke={c.background}
                        strokeWidth={1.6}
                        strokeLinejoin="round"
                        strokeLinecap="round"
                    />
                </Svg>
            </Pressable>
        </View>
    );
};

const styles = StyleSheet.create({
    row: { flexDirection: 'row', alignItems: 'center', gap: 8 },
    input: {
        flex: 1,
        borderWidth: 1,
        borderRadius: 14,
        paddingHorizontal: 14,
        paddingVertical: 11,
        minHeight: 44,
        fontSize: 15,
    },
    send: { width: 42, height: 42, borderRadius: 14, alignItems: 'center', justifyContent: 'center' },
});
