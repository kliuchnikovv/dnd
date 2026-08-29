import {
    defaultMotion,
    defaultRadius,
    defaultSpacing,
    defaultTypography,
} from './tokens';
import { ThemeDescriptor, ThemePalette } from './types';

// Inspired by the colorful retro pastel palette
// Dark charcoal, brown, coral, salmon, peach, olive green, teal, and indigo
const palette: ThemePalette = {
    red: '#E94B4B', // Vibrant coral red
    orange: '#F08E7A', // Salmon/peach
    yellow: '#F5C79A', // Light warm peach
    green: '#6B8E50', // Olive green
    cyan: '#4A9B9B', // Teal
    blue: '#4A9B9B', // Teal (used for blue role)
    magenta: '#6B5B95', // Indigo/purple
    purple: '#6B5B95', // Same as magenta
    black: '#2D2D36', // Dark charcoal
    white: '#FAF0E6', // Cream/linen background
    fg: '#2D2D36', // Dark text on light
    bg: '#FAF0E6', // Cream background
    gutterGrey: '#8B6955', // Brown accent
    commentGrey: '#A89080', // Muted brown
    // Additional colors for chat UI
    background: '#FAF0E6', // Cream/linen
    foreground: '#2D2D36', // Dark charcoal
    currentLine: '#F5EDE0', // Slightly darker cream
    selection: '#F5C79A', // Warm peach selection
    comment: '#8B6955', // Brown for comments
};

export const retroPastelTheme: ThemeDescriptor = {
    id: 'retroPastel',
    name: 'Retro Pastel',
    variant: 'light',
    palette: palette,
    colors: {
        attention: palette.red,
        background: palette.background,
        icon: palette.magenta,
        text: palette.black,
        fadedText: 'rgba(45, 45, 54, 0.6)',
        accent: palette.cyan,
        accentAlt: palette.green,
        border: '#E0D5C8',
        textSecondary: palette.gutterGrey,
        textPrimary: palette.fg,
        surface: '#FFFFFF',
        surfaceAlt: '#F5EDE0',
        surfaceHighlight: palette.selection ?? '#F5C79A',
        primary: palette.cyan,
        error: palette.red,
        card: '#FFFFFF',
        // Extended semantic colors
        warning: palette.orange,
        success: palette.green,
        info: palette.cyan,
        // Source-specific colors
        calendar: palette.magenta,
        email: palette.red,
        task: palette.green,
        // Code colors
        codeBg: palette.currentLine,
        codeInlineBg: '#FFFFFF',
        codeText: palette.fg,
        // Blockquote colors
        blockquoteBg: '#F5EDE0', // surfaceAlt
        blockquoteBorder: palette.cyan, // accent
    },
    spacing: defaultSpacing,
    radius: defaultRadius,
    typography: defaultTypography,
    motion: defaultMotion,
};

export const retroPastel = retroPastelTheme.colors;
