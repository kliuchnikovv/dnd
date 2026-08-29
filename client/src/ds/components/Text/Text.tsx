import React from 'react';
import { Text as RNText, StyleSheet } from 'react-native';
import type { StyleProp, TextProps as RNTextProps, TextStyle } from 'react-native';

import type { ThemeColors, ThemeDescriptor, TypographyVariant } from '@genie/front/themes/types';

export type TextVariant = keyof ThemeDescriptor['typography'];

export type TextTone = 'primary' | 'secondary' | 'faded' | 'accent' | 'error' | 'success' | 'warning';

export type TextProps = Omit<RNTextProps, 'style'> & {
    theme: ThemeDescriptor;
    variant?: TextVariant;
    tone?: TextTone;
    align?: TextStyle['textAlign'];
    weight?: TypographyVariant['fontWeight'];
    style?: StyleProp<TextStyle>;
};

const toneToColor = (tone: TextTone, colors: ThemeColors): string => {
    switch (tone) {
        case 'primary':
            return colors.textPrimary;
        case 'secondary':
            return colors.textSecondary;
        case 'faded':
            return colors.fadedText;
        case 'accent':
            return colors.accent;
        case 'error':
            return colors.error ?? colors.attention;
        case 'success':
            return colors.success ?? colors.accent;
        case 'warning':
            return colors.warning ?? colors.accent;
    }
};

export const Text: React.FC<TextProps> = ({
    theme,
    variant = 'body',
    tone = 'primary',
    align,
    weight,
    style,
    ...rest
}) => {
    const v = theme.typography[variant];
    const composed: TextStyle = {
        fontFamily: v.fontFamily,
        fontSize: v.fontSize,
        lineHeight: v.lineHeight,
        fontWeight: weight ?? v.fontWeight,
        color: toneToColor(tone, theme.colors),
    };
    if (v.letterSpacing !== undefined) {
        composed.letterSpacing = v.letterSpacing;
    }
    if (align !== undefined) {
        composed.textAlign = align;
    }
    return <RNText {...rest} style={StyleSheet.flatten([composed, style])} />;
};
