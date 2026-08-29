import React, { forwardRef, useState } from 'react';
import { StyleSheet, TextInput, View } from 'react-native';
import type { StyleProp, TextInputProps, TextStyle, ViewStyle } from 'react-native';

import type { ThemeDescriptor } from '@genie/front/themes/types';

import { Text } from '../Text';

export type InputSize = 'sm' | 'md' | 'lg';

export type InputProps = Omit<TextInputProps, 'style'> & {
    theme: ThemeDescriptor;
    size?: InputSize;
    label?: string;
    hint?: string;
    error?: string;
    leftAdornment?: React.ReactNode;
    rightAdornment?: React.ReactNode;
    containerStyle?: StyleProp<ViewStyle>;
    inputStyle?: StyleProp<TextStyle>;
};

type SizeSpec = {
    paddingV: number;
    paddingH: number;
    minHeight: number;
    gap: number;
    textVariant: 'body' | 'caption';
};

const sizeFor = (size: InputSize, theme: ThemeDescriptor): SizeSpec => {
    const sp = theme.spacing;
    switch (size) {
        case 'sm':
            return { paddingV: sp.xs, paddingH: sp.sm, minHeight: 32, gap: sp.xs, textVariant: 'caption' };
        case 'md':
            return { paddingV: sp.sm, paddingH: sp.md, minHeight: 40, gap: sp.sm, textVariant: 'body' };
        case 'lg':
            return { paddingV: sp.md, paddingH: sp.lg, minHeight: 48, gap: sp.sm, textVariant: 'body' };
    }
};

export const Input = forwardRef<TextInput, InputProps>(function Input(
    {
        theme,
        size = 'md',
        label,
        hint,
        error,
        leftAdornment,
        rightAdornment,
        containerStyle,
        inputStyle,
        editable,
        onFocus,
        onBlur,
        placeholderTextColor,
        ...rest
    },
    ref,
) {
    const [focused, setFocused] = useState(false);
    const s = sizeFor(size, theme);
    const disabled = editable === false;
    const hasError = Boolean(error);

    const errorColor = theme.colors.error ?? theme.colors.attention;
    const accent = theme.colors.primary ?? theme.colors.accent;

    const borderColor = hasError
        ? errorColor
        : focused
            ? accent
            : theme.colors.border;

    const fieldStyle: ViewStyle = {
        flexDirection: 'row',
        alignItems: 'center',
        paddingVertical: s.paddingV,
        paddingHorizontal: s.paddingH,
        minHeight: s.minHeight,
        borderRadius: theme.radius.md,
        borderWidth: 1,
        borderColor,
        backgroundColor: disabled ? theme.colors.surfaceAlt : theme.colors.surface,
        opacity: disabled ? 0.7 : 1,
    };

    const textInputStyle: TextStyle = {
        flex: 1,
        color: theme.colors.textPrimary,
        fontFamily: theme.typography.body.fontFamily,
        fontSize: s.textVariant === 'caption' ? theme.typography.caption.fontSize : theme.typography.body.fontSize,
        padding: 0,
    };

    return (
        <View style={containerStyle}>
            {label !== undefined ? (
                <Text theme={theme} variant="caption" tone="secondary" style={{ marginBottom: theme.spacing.xs }}>
                    {label}
                </Text>
            ) : null}
            <View style={fieldStyle}>
                {leftAdornment !== undefined ? (
                    <View style={{ marginRight: s.gap }}>{leftAdornment}</View>
                ) : null}
                <TextInput
                    ref={ref}
                    editable={editable}
                    {...rest}
                    placeholderTextColor={placeholderTextColor ?? theme.colors.fadedText}
                    onFocus={(e) => {
                        setFocused(true);
                        onFocus?.(e);
                    }}
                    onBlur={(e) => {
                        setFocused(false);
                        onBlur?.(e);
                    }}
                    style={StyleSheet.flatten([textInputStyle, inputStyle])}
                />
                {rightAdornment !== undefined ? (
                    <View style={{ marginLeft: s.gap }}>{rightAdornment}</View>
                ) : null}
            </View>
            {hasError ? (
                <Text theme={theme} variant="caption" tone="error" style={{ marginTop: theme.spacing.xs }}>
                    {error}
                </Text>
            ) : hint !== undefined ? (
                <Text theme={theme} variant="caption" tone="faded" style={{ marginTop: theme.spacing.xs }}>
                    {hint}
                </Text>
            ) : null}
        </View>
    );
});
