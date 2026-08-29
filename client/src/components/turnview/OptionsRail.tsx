import React from 'react';
import { Pressable, ScrollView, StyleSheet, View } from 'react-native';

import { Text } from '@genie/ds';
import { useTheme } from '../../theme/ThemeContext';
import { Option } from '../../turnview/types';
import { Intent, tokenIntent } from '../../turnview/intents';

// OptionsRail — предложенные ходы. Тап шлёт непрозрачный Option.token как есть; клиент интент
// не сочиняет и токен не читает. check — класс проверки (тег-pill, НЕ порог). reply — реплика
// (берём в кавычки). layout: 'list' (диалог, нумерованный столбец) | 'rail' (сцена, лента чипов).

export type OptionsLayout = 'list' | 'rail';

export const OptionsRail: React.FC<{
    options?: Option[];
    onIntent: (intent: Intent) => void;
    layout?: OptionsLayout;
}> = ({ options, onIntent, layout = 'list' }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    if (!options || options.length === 0) return null;

    const items = options.map((o, i) => (
        <OptionItem key={o.id} option={o} index={i} layout={layout} onPress={() => onIntent(tokenIntent(o.token))} />
    ));

    if (layout === 'rail') {
        return (
            <ScrollView
                horizontal
                showsHorizontalScrollIndicator={false}
                contentContainerStyle={styles.rail}
            >
                {items}
            </ScrollView>
        );
    }
    return <View style={styles.list}>{items}</View>;
};

const OptionItem: React.FC<{
    option: Option;
    index: number;
    layout: OptionsLayout;
    onPress: () => void;
}> = ({ option, index, layout, onPress }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const label = option.reply ? `«${option.label}»` : option.label;

    return (
        <Pressable
            onPress={onPress}
            hitSlop={8}
            style={({ pressed }) => [
                layout === 'rail' ? styles.chip : styles.rowItem,
                {
                    borderColor: c.stroke ?? c.border,
                    backgroundColor: pressed ? c.surfaceHighlight : c.surface,
                    opacity: pressed ? 0.85 : 1,
                },
            ]}
        >
            {layout === 'list' ? (
                <Text theme={theme} variant="mono" style={[styles.num, { color: c.inkFaint }]}>
                    {index + 1}
                </Text>
            ) : null}
            <Text
                theme={theme}
                variant="body"
                style={[
                    styles.label,
                    { color: c.text, fontStyle: option.reply ? 'italic' : 'normal' },
                ]}
            >
                {label}
            </Text>
            {option.check ? (
                <View style={[styles.checkPill, { backgroundColor: c.amberFill }]}>
                    <Text theme={theme} variant="mono" style={[styles.checkText, { color: c.amberText }]}>
                        {option.check.toUpperCase()}
                    </Text>
                </View>
            ) : null}
        </Pressable>
    );
};

const styles = StyleSheet.create({
    list: { gap: 8 },
    rail: { gap: 8, paddingVertical: 2 },
    rowItem: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 10,
        borderWidth: 1,
        borderRadius: 13,
        paddingVertical: 11,
        paddingHorizontal: 13,
        minHeight: 44,
    },
    chip: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 8,
        borderWidth: 1,
        borderRadius: 999,
        paddingVertical: 9,
        paddingHorizontal: 14,
        minHeight: 40,
    },
    num: { fontSize: 11, width: 14, textAlign: 'center' },
    label: { flexShrink: 1 },
    checkPill: { borderRadius: 999, paddingVertical: 3, paddingHorizontal: 8 },
    checkText: { fontSize: 9, letterSpacing: 0.8 },
});
