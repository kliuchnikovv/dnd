import React from 'react';
import {
    ActivityIndicator,
    Pressable,
    StyleSheet,
    View,
} from 'react-native';
import type {
    PressableProps,
    StyleProp,
    TextStyle,
    ViewStyle,
} from 'react-native';

import type { ThemeDescriptor } from '@genie/front/themes/types';

import { Text } from '../Text';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
export type ButtonSize = 'sm' | 'md' | 'lg';

export type ButtonProps = Omit<PressableProps, 'style' | 'children'> & {
    theme: ThemeDescriptor;
    label: string;
    variant?: ButtonVariant;
    size?: ButtonSize;
    loading?: boolean;
    fullWidth?: boolean;
    leftIcon?: React.ReactNode;
    rightIcon?: React.ReactNode;
    style?: StyleProp<ViewStyle>;
    labelStyle?: StyleProp<TextStyle>;
};

type VisualSpec = {
    bg: string;
    bgPressed: string;
    bgDisabled: string;
    fg: string;
    fgDisabled: string;
    borderColor: string | undefined;
    borderWidth: number;
};

const visualFor = (variant: ButtonVariant, theme: ThemeDescriptor): VisualSpec => {
    const { colors } = theme;
    const primary = colors.primary ?? colors.accent;
    const danger = colors.error ?? colors.attention;
    switch (variant) {
        case 'primary':
            return {
                bg: primary,
                bgPressed: primary,
                bgDisabled: colors.surfaceAlt,
                fg: colors.background,
                fgDisabled: colors.fadedText,
                borderColor: undefined,
                borderWidth: 0,
            };
        case 'secondary':
            return {
                bg: colors.surface,
                bgPressed: colors.surfaceHighlight,
                bgDisabled: colors.surfaceAlt,
                fg: colors.textPrimary,
                fgDisabled: colors.fadedText,
                borderColor: colors.border,
                borderWidth: 1,
            };
        case 'ghost':
            return {
                bg: 'transparent',
                bgPressed: colors.surfaceHighlight,
                bgDisabled: 'transparent',
                fg: primary,
                fgDisabled: colors.fadedText,
                borderColor: undefined,
                borderWidth: 0,
            };
        case 'danger':
            return {
                bg: danger,
                bgPressed: danger,
                bgDisabled: colors.surfaceAlt,
                fg: colors.background,
                fgDisabled: colors.fadedText,
                borderColor: undefined,
                borderWidth: 0,
            };
    }
};

type SizeSpec = {
    paddingV: number;
    paddingH: number;
    minHeight: number;
    gap: number;
    textVariant: 'body' | 'caption';
};

const sizeFor = (size: ButtonSize, theme: ThemeDescriptor): SizeSpec => {
    const { spacing } = theme;
    switch (size) {
        case 'sm':
            return { paddingV: spacing.xs, paddingH: spacing.md, minHeight: 32, gap: spacing.xs, textVariant: 'caption' };
        case 'md':
            return { paddingV: spacing.sm, paddingH: spacing.lg, minHeight: 44, gap: spacing.sm, textVariant: 'body' };
        case 'lg':
            return { paddingV: spacing.md, paddingH: spacing.xl, minHeight: 52, gap: spacing.sm, textVariant: 'body' };
    }
};

export const Button: React.FC<ButtonProps> = ({
    theme,
    label,
    variant = 'primary',
    size = 'md',
    loading = false,
    fullWidth = false,
    leftIcon,
    rightIcon,
    disabled,
    style,
    labelStyle,
    ...rest
}) => {
    const v = visualFor(variant, theme);
    const s = sizeFor(size, theme);
    const isDisabled = disabled || loading;

    return (
        <Pressable
            {...rest}
            disabled={isDisabled}
            style={({ pressed }) => {
                const composed: ViewStyle = {
                    flexDirection: 'row',
                    alignItems: 'center',
                    justifyContent: 'center',
                    paddingVertical: s.paddingV,
                    paddingHorizontal: s.paddingH,
                    minHeight: s.minHeight,
                    borderRadius: theme.radius.md,
                    backgroundColor: isDisabled ? v.bgDisabled : pressed ? v.bgPressed : v.bg,
                    borderWidth: v.borderWidth,
                    opacity: pressed && !isDisabled ? 0.9 : 1,
                    alignSelf: fullWidth ? 'stretch' : 'flex-start',
                };
                if (v.borderColor !== undefined) {
                    composed.borderColor = v.borderColor;
                }
                return StyleSheet.flatten([composed, style]);
            }}
        >
            {loading ? (
                <ActivityIndicator size="small" color={v.fg} />
            ) : (
                <>
                    {leftIcon !== undefined ? (
                        <View style={{ marginRight: s.gap }}>{leftIcon}</View>
                    ) : null}
                    <Text
                        theme={theme}
                        variant={s.textVariant}
                        weight="600"
                        style={StyleSheet.flatten([{ color: isDisabled ? v.fgDisabled : v.fg }, labelStyle])}
                    >
                        {label}
                    </Text>
                    {rightIcon !== undefined ? (
                        <View style={{ marginLeft: s.gap }}>{rightIcon}</View>
                    ) : null}
                </>
            )}
        </Pressable>
    );
};
