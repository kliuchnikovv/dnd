import React from 'react';
import { Pressable, StyleSheet, View } from 'react-native';
import type { PressableProps, StyleProp, ViewStyle } from 'react-native';

import type { ThemeDescriptor } from '@genie/front/themes/types';

import { Text } from '../Text';

export type ListItemDensity = 'compact' | 'comfortable';

export type ListItemProps = Omit<PressableProps, 'style' | 'children'> & {
    theme: ThemeDescriptor;
    title: string;
    subtitle?: string;
    leftContent?: React.ReactNode;
    rightContent?: React.ReactNode;
    density?: ListItemDensity;
    selected?: boolean;
    style?: StyleProp<ViewStyle>;
};

const paddingFor = (density: ListItemDensity, theme: ThemeDescriptor) => {
    const s = theme.spacing;
    return density === 'compact'
        ? { paddingV: s.sm, paddingH: s.md, gap: s.sm }
        : { paddingV: s.md, paddingH: s.lg, gap: s.md };
};

export const ListItem: React.FC<ListItemProps> = ({
    theme,
    title,
    subtitle,
    leftContent,
    rightContent,
    density = 'comfortable',
    selected = false,
    onPress,
    disabled,
    style,
    ...rest
}) => {
    const p = paddingFor(density, theme);
    const pressable = onPress !== undefined && !disabled;

    const rowStyle = (pressed: boolean): ViewStyle => ({
        flexDirection: 'row',
        alignItems: 'center',
        paddingVertical: p.paddingV,
        paddingHorizontal: p.paddingH,
        backgroundColor: selected
            ? theme.colors.surfaceHighlight
            : pressed && pressable
                ? theme.colors.surfaceAlt
                : 'transparent',
        opacity: disabled ? 0.5 : 1,
    });

    const content = (
        <>
            {leftContent !== undefined ? (
                <View style={{ marginRight: p.gap }}>{leftContent}</View>
            ) : null}
            <View style={{ flex: 1 }}>
                <Text theme={theme} variant="body" tone="primary" weight="500">
                    {title}
                </Text>
                {subtitle !== undefined ? (
                    <Text theme={theme} variant="caption" tone="secondary" style={{ marginTop: 2 }}>
                        {subtitle}
                    </Text>
                ) : null}
            </View>
            {rightContent !== undefined ? (
                <View style={{ marginLeft: p.gap }}>{rightContent}</View>
            ) : null}
        </>
    );

    if (!pressable) {
        return <View style={StyleSheet.flatten([rowStyle(false), style])}>{content}</View>;
    }
    return (
        <Pressable
            {...rest}
            onPress={onPress}
            disabled={disabled}
            style={({ pressed }) => StyleSheet.flatten([rowStyle(pressed), style])}
        >
            {content}
        </Pressable>
    );
};
