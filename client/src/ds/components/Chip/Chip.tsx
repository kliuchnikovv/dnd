import React from 'react';
import { Pressable, StyleSheet, View } from 'react-native';
import type { PressableProps, StyleProp, ViewStyle } from 'react-native';

import type { ThemeColors, ThemeDescriptor } from '@genie/front/themes/types';

import { Text } from '../Text';

export type ChipTone = 'default' | 'accent' | 'success' | 'error' | 'warning';
export type ChipVariant = 'solid' | 'soft' | 'outlined';
export type ChipSize = 'sm' | 'md';

export type ChipProps = Omit<PressableProps, 'style' | 'children'> & {
    theme: ThemeDescriptor;
    label: string;
    tone?: ChipTone;
    variant?: ChipVariant;
    size?: ChipSize;
    selected?: boolean;
    leftIcon?: React.ReactNode;
    rightIcon?: React.ReactNode;
    style?: StyleProp<ViewStyle>;
};

const toneBase = (tone: ChipTone, colors: ThemeColors): string => {
    switch (tone) {
        case 'default':
            return colors.textPrimary;
        case 'accent':
            return colors.primary ?? colors.accent;
        case 'success':
            return colors.success ?? colors.accent;
        case 'error':
            return colors.error ?? colors.attention;
        case 'warning':
            return colors.warning ?? colors.accent;
    }
};

type SizeSpec = { paddingV: number; paddingH: number; gap: number; textVariant: 'body' | 'caption' };

const sizeFor = (size: ChipSize, theme: ThemeDescriptor): SizeSpec => {
    const s = theme.spacing;
    return size === 'sm'
        ? { paddingV: s.xs, paddingH: s.sm, gap: s.xs, textVariant: 'caption' }
        : { paddingV: s.sm, paddingH: s.md, gap: s.xs, textVariant: 'body' };
};

export const Chip: React.FC<ChipProps> = ({
    theme,
    label,
    tone = 'default',
    variant = 'soft',
    size = 'sm',
    selected = false,
    leftIcon,
    rightIcon,
    onPress,
    disabled,
    style,
    ...rest
}) => {
    const base = toneBase(tone, theme.colors);
    const s = sizeFor(size, theme);
    const pressable = onPress !== undefined && !disabled;

    const surface = variant === 'solid'
        ? base
        : variant === 'soft'
            ? (tone === 'default' ? theme.colors.surface : `${base}22`)
            : 'transparent';
    const fg = variant === 'solid' ? theme.colors.background : base;
    const borderColor = variant === 'outlined' ? base : selected ? base : theme.colors.border;
    const borderWidth = variant === 'outlined' || selected ? 1 : 0;

    const containerStyle = (pressed: boolean): ViewStyle => ({
        flexDirection: 'row',
        alignItems: 'center',
        paddingVertical: s.paddingV,
        paddingHorizontal: s.paddingH,
        borderRadius: theme.radius.pill,
        backgroundColor: surface,
        borderWidth,
        borderColor,
        opacity: disabled ? 0.5 : pressed && pressable ? 0.85 : 1,
        alignSelf: 'flex-start',
    });

    const content = (
        <>
            {leftIcon !== undefined ? <View style={{ marginRight: s.gap }}>{leftIcon}</View> : null}
            <Text theme={theme} variant={s.textVariant} weight="500" style={{ color: fg }}>
                {label}
            </Text>
            {rightIcon !== undefined ? <View style={{ marginLeft: s.gap }}>{rightIcon}</View> : null}
        </>
    );

    if (!pressable) {
        return <View style={StyleSheet.flatten([containerStyle(false), style])}>{content}</View>;
    }
    return (
        <Pressable
            {...rest}
            onPress={onPress}
            disabled={disabled}
            style={({ pressed }) => StyleSheet.flatten([containerStyle(pressed), style])}
        >
            {content}
        </Pressable>
    );
};
