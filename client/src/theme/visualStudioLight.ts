import {
    defaultMotion,
    defaultRadius,
    defaultSpacing,
    defaultTypography,
} from './tokens';
import { ThemeDescriptor, ThemePalette } from './types';

const palette: ThemePalette = {
    red: '#a31515',
    orange: '#d16969',
    yellow: '#795e26',
    green: '#098658',
    cyan: '#0451a5',
    blue: '#0000ff',
    magenta: '#811f3f',
    purple: '#af00db', // Purple color for VS Light
    white: '#ffffff',
    fg: '#000000',
    black: '#000000',
    bg: '#ffffff',
    gutterGrey: '#f3f3f3',
    commentGrey: '#008000',
    selection: '#add6ff',
    // Additional colors for chat UI
    background: '#ffffff',
    foreground: '#000000',
    currentLine: '#eff284',
    comment: '#008000',
};

export const visualStudioLightTheme: ThemeDescriptor = {
    id: 'visualStudioLight',
    name: 'Light (Visual Studio)',
    variant: 'light',
    palette: palette,
    colors: {
        attention: palette.red,
        background: palette.bg,
        icon: palette.fg,
        text: palette.fg,
        fadedText: 'rgba(0, 0, 0, 0.7)',
        accent: palette.blue,
        accentAlt: palette.cyan,
        border: '#e5e5e5',
        textSecondary: '#666666',
        textPrimary: palette.fg,
        surface: '#f3f3f3',
        surfaceAlt: '#e5e5e5',
        surfaceHighlight: '#d4d4d4',
        primary: palette.blue,
        error: palette.red,
        card: '#ffffff',
        // Extended semantic colors
        warning: '#d97706',
        success: palette.green,
        info: palette.cyan,
        // Source-specific colors
        calendar: '#4285F4',
        email: '#EA4335',
        task: '#34A853',
        // Blockquote colors
        blockquoteBg: '#f3f3f3', // gutterGrey - subtle highlight
        blockquoteBorder: palette.cyan, // #0451a5 - accent for emphasis
    },
    spacing: defaultSpacing,
    radius: defaultRadius,
    typography: defaultTypography,
    motion: defaultMotion,
};

export const visualStudioLight = visualStudioLightTheme.colors;
