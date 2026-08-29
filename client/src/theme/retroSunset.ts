import {
    defaultMotion,
    defaultRadius,
    defaultSpacing,
    defaultTypography,
} from './tokens';
import { ThemeDescriptor, ThemePalette } from './types';

const palette: ThemePalette = {
    red: '#E95851', // Warm coral red
    orange: '#E6836A', // Soft peach orange
    yellow: '#D6C9A8', // Warm cream/beige
    green: '#8FB8B5', // Muted sage green-teal
    cyan: '#399CA7', // Vibrant teal
    blue: '#195F7A', // Deep teal blue
    magenta: '#E6836A', // Peachy tone
    purple: '#B5838D', // Muted purple
    black: '#0F2A3A', // Very deep navy
    white: '#F5F0E6', // Warm off-white
    fg: '#F5F0E6',
    bg: '#163D53', // Deep navy background
    gutterGrey: '#2A5669', // Muted teal-grey
    commentGrey: '#5D8C8A', // Soft teal grey
    // Additional colors for chat UI
    background: '#163D53',
    foreground: '#F5F0E6',
    currentLine: '#1C4A62',
    selection: '#2A5669',
    comment: '#5D8C8A',
};

export const retroSunsetTheme: ThemeDescriptor = {
    id: 'retroSunset',
    name: 'Retro Sunset',
    variant: 'dark',
    palette: palette,
    colors: {
        attention: palette.red,
        background: palette.background,
        icon: palette.white,
        text: palette.white,
        fadedText: 'rgba(245, 240, 230, 0.7)',
        accent: palette.cyan,
        accentAlt: palette.orange,
        border: palette.gutterGrey,
        textSecondary: palette.commentGrey,
        textPrimary: palette.fg,
        surface: '#1C4A62',
        surfaceAlt: '#0F2A3A',
        surfaceHighlight: palette.selection ?? '#2A5669',
        primary: palette.cyan,
        error: palette.red,
        card: '#1F4F68',
        // Extended semantic colors
        warning: palette.orange,
        success: palette.green,
        info: palette.cyan,
        // Source-specific colors
        calendar: palette.cyan,
        email: palette.red,
        task: palette.green,
        // Code colors
        codeBg: palette.currentLine,
        codeInlineBg: '#1C4A62',
        codeText: palette.fg,
        // Blockquote colors
        blockquoteBg: '#0F2A3A', // surfaceAlt
        blockquoteBorder: palette.cyan, // accent
    },
    spacing: defaultSpacing,
    radius: defaultRadius,
    typography: defaultTypography,
    motion: defaultMotion,
};

export const retroSunset = retroSunsetTheme.colors;
