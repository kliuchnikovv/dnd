import React from 'react';
import { ActivityIndicator } from 'react-native';
import type { StyleProp, ViewStyle } from 'react-native';

import type { ThemeDescriptor } from '@genie/front/themes/types';

export type SpinnerSize = 'sm' | 'md' | 'lg';
export type SpinnerTone = 'primary' | 'accent' | 'onAccent';

export type SpinnerProps = {
    theme: ThemeDescriptor;
    size?: SpinnerSize;
    tone?: SpinnerTone;
    color?: string;
    style?: StyleProp<ViewStyle>;
};

const sizeToNumber = (size: SpinnerSize): number => {
    switch (size) {
        case 'sm':
            return 16;
        case 'md':
            return 24;
        case 'lg':
            return 36;
    }
};

const toneToColor = (tone: SpinnerTone, theme: ThemeDescriptor): string => {
    switch (tone) {
        case 'primary':
            return theme.colors.textPrimary;
        case 'accent':
            return theme.colors.primary ?? theme.colors.accent;
        case 'onAccent':
            return theme.colors.background;
    }
};

export const Spinner: React.FC<SpinnerProps> = ({
    theme,
    size = 'md',
    tone = 'accent',
    color,
    style,
}) => (
    <ActivityIndicator
        size={sizeToNumber(size)}
        color={color ?? toneToColor(tone, theme)}
        style={style}
    />
);
