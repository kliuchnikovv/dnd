import {
    defaultMotion,
    defaultRadius,
    defaultSpacing,
    defaultTypography,
} from './tokens';
import { ThemeDescriptor, ThemePalette } from './types';

const palette: ThemePalette = {
    red: '#f07178',
    orange: '#ffae57',
    yellow: '#f6c177',
    green: '#aad94c',
    cyan: '#95e1d3',
    blue: '#59c2c9',
    magenta: '#d2a6ff',
    purple: '#d2a6ff', // Same as magenta for now
    white: '#e6e6e6',
    fg: '#b8c5d0',
    black: '#0f2027',
    bg: '#0f2027',
    gutterGrey: '#1a3a3f',
    commentGrey: '#526a72',
    selection: '#1e4d53',
    // Additional colors for chat UI
    background: '#0f2027',
    foreground: '#b8c5d0',
    currentLine: '#1a3a3f',
    comment: '#526a72',
};

export const deepOceanTheme: ThemeDescriptor = {
    id: 'deepOcean',
    name: 'Deep Ocean',
    variant: 'dark',
    palette: palette,
    colors: {
        attention: palette.cyan,
        background: palette.bg,
        icon: palette.cyan,
        text: palette.fg,
        fadedText: 'rgba(184, 197, 208, 0.6)',
        accent: palette.cyan,
        accentAlt: palette.blue,
        border: palette.gutterGrey,
        textSecondary: palette.commentGrey,
        textPrimary: palette.fg,
        surface: '#1a3a3f',
        surfaceAlt: '#152e33',
        surfaceHighlight: '#24555c',
        primary: palette.cyan,
        error: palette.red,
        card: '#1a3a3f',
        // Extended semantic colors
        warning: palette.orange,
        success: palette.green,
        info: palette.blue,
        // Source-specific colors
        calendar: palette.blue,
        email: palette.magenta,
        task: palette.green,
        // Code colors
        codeBg: '#152e33', // darker surface for code blocks
        codeInlineBg: '#1a3a3f', // surface
        codeText: palette.fg, // readable code color
        // Blockquote colors
        blockquoteBg: '#152e33', // surfaceAlt
        blockquoteBorder: palette.cyan, // accent
    },
    spacing: defaultSpacing,
    radius: defaultRadius,
    typography: defaultTypography,
    motion: defaultMotion,
};

export const deepOcean = deepOceanTheme.colors;
export const deepOceanPalette = palette;
