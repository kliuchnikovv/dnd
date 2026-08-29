import React from 'react';
import { StyleSheet, View } from 'react-native';
import type { StyleProp, ViewStyle } from 'react-native';

import type { ThemeDescriptor } from '@genie/front/themes/types';

export type DividerOrientation = 'horizontal' | 'vertical';
export type DividerInset = keyof ThemeDescriptor['spacing'] | 'none';

export type DividerProps = {
    theme: ThemeDescriptor;
    orientation?: DividerOrientation;
    inset?: DividerInset;
    thickness?: number;
    color?: string;
    style?: StyleProp<ViewStyle>;
};

export const Divider: React.FC<DividerProps> = ({
    theme,
    orientation = 'horizontal',
    inset = 'none',
    thickness = StyleSheet.hairlineWidth,
    color,
    style,
}) => {
    const insetValue = inset === 'none' ? 0 : theme.spacing[inset];
    const base: ViewStyle =
        orientation === 'horizontal'
            ? { height: thickness, alignSelf: 'stretch', marginHorizontal: insetValue }
            : { width: thickness, alignSelf: 'stretch', marginVertical: insetValue };
    base.backgroundColor = color ?? theme.colors.border;
    return <View style={StyleSheet.flatten([base, style])} />;
};
