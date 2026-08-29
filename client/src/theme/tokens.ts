import { Platform } from 'react-native';

import type { ThemeMotion, ThemeRadius, ThemeSpacing, ThemeTypography } from './types';

export const defaultSpacing: ThemeSpacing = {
    xs: 4,
    sm: 8,
    md: 12,
    lg: 16,
    xl: 24,
    xxl: 32,
};

export const defaultRadius: ThemeRadius = {
    sm: 6,
    md: 10,
    lg: 16,
    xl: 24,
    pill: 999,
};

const systemFont = Platform.select({ ios: 'System', android: 'sans-serif', default: 'System' });
const systemMono = Platform.select({ ios: 'Menlo', android: 'monospace', default: 'monospace' });

export const defaultTypography: ThemeTypography = {
    display: { fontFamily: systemFont, fontSize: 32, lineHeight: 38, fontWeight: '700' },
    title:   { fontFamily: systemFont, fontSize: 22, lineHeight: 28, fontWeight: '600' },
    body:    { fontFamily: systemFont, fontSize: 16, lineHeight: 22, fontWeight: '400' },
    caption: { fontFamily: systemFont, fontSize: 12, lineHeight: 16, fontWeight: '400' },
    mono:    { fontFamily: systemMono, fontSize: 14, lineHeight: 20, fontWeight: '400' },
};

export const defaultMotion: ThemeMotion = {
    fast: 120,
    base: 200,
    slow: 320,
};
