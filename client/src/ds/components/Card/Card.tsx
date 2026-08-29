import React from 'react';
import { StyleSheet, View } from 'react-native';
import type { StyleProp, ViewProps, ViewStyle } from 'react-native';

import type { ThemeDescriptor } from '@genie/front/themes/types';

export type CardPadding = keyof ThemeDescriptor['spacing'] | 'none';
export type CardRadius = keyof ThemeDescriptor['radius'];
export type CardSurface = 'surface' | 'surfaceAlt' | 'surfaceHighlight' | 'transparent';
export type CardElevation = 'none' | 'raised' | 'inset';

export type CardProps = Omit<ViewProps, 'style'> & {
    theme: ThemeDescriptor;
    padding?: CardPadding;
    radius?: CardRadius;
    surface?: CardSurface;
    bordered?: boolean;
    elevation?: CardElevation;
    style?: StyleProp<ViewStyle>;
};

const surfaceColor = (surface: CardSurface, theme: ThemeDescriptor): string => {
    switch (surface) {
        case 'surface':
            return theme.colors.surface;
        case 'surfaceAlt':
            return theme.colors.surfaceAlt;
        case 'surfaceHighlight':
            return theme.colors.surfaceHighlight;
        case 'transparent':
            return 'transparent';
    }
};

export const Card: React.FC<CardProps> = ({
    theme,
    padding = 'lg',
    radius = 'lg',
    surface = 'surface',
    bordered = false,
    elevation = 'none',
    style,
    children,
    ...rest
}) => {
    const composed: ViewStyle = {
        backgroundColor: surfaceColor(surface, theme),
        borderRadius: theme.radius[radius],
    };
    if (padding !== 'none') {
        composed.padding = theme.spacing[padding];
    }
    if (bordered) {
        composed.borderWidth = 1;
        composed.borderColor = theme.colors.border;
    }
    if (elevation !== 'none' && theme.effects !== undefined) {
        const shadow = elevation === 'raised' ? theme.effects.cardShadowRaised : theme.effects.cardShadowInset;
        composed.shadowColor = shadow.shadowColor;
        composed.shadowOffset = shadow.shadowOffset;
        composed.shadowOpacity = shadow.shadowOpacity;
        composed.shadowRadius = shadow.shadowRadius;
        if (shadow.elevation !== undefined) {
            composed.elevation = shadow.elevation;
        }
    }
    return (
        <View {...rest} style={StyleSheet.flatten([composed, style])}>
            {children}
        </View>
    );
};
