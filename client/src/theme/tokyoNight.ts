import {
    defaultMotion,
    defaultRadius,
    defaultSpacing,
    defaultTypography,
} from './tokens';
import { ThemeDescriptor, ThemePalette } from './types';

const palette: ThemePalette = {
    red: '#f7768e',
    orange: '#ff9e64',
    yellow: '#e0af68',
    green: '#9ece6a',
    cyan: '#7dcfff',
    blue: '#7aa2f7',
    magenta: '#bb9af7',
    purple: '#bb9af7', // Same as magenta for now
    white: '#a9b1d6',
    fg: '#c0caf5',
    black: '#414868',
    bg: '#1a1b26',
    gutterGrey: '#3b4261',
    commentGrey: '#565f89',
    selection: '#364a82',
    // Additional colors for chat UI
    background: '#1a1b26',
    foreground: '#c0caf5',
    currentLine: '#292e42',
    comment: '#565f89',
};

export const tokyoNightTheme: ThemeDescriptor = {
    id: 'tokyoNight',
    name: 'Tokyo Night',
    variant: 'dark',
    palette: palette,
    colors: {
        attention: palette.red,
        background: palette.bg,
        icon: palette.fg,
        text: palette.fg,
        fadedText: 'rgba(192, 202, 245, 0.7)',
        accent: palette.blue,
        accentAlt: palette.cyan,
        border: palette.gutterGrey,
        textSecondary: palette.commentGrey,
        textPrimary: palette.fg,
        surface: '#24283b',
        surfaceAlt: '#1f2335',
        surfaceHighlight: '#2f3549',
        primary: palette.blue,
        error: palette.red,
        card: '#24283b',
        // Extended semantic colors
        warning: palette.orange,
        success: palette.green,
        info: palette.cyan,
        // Source-specific colors
        calendar: palette.blue,
        email: palette.red,
        task: palette.green,
        // Code colors
        codeBg: '#292e42', // currentLine - lighter than bg for visibility
        codeInlineBg: '#24283b', // surface
        codeText: palette.fg, // #c0caf5 - readable code color
        // Blockquote colors
        blockquoteBg: '#1f2335', // surfaceAlt - subtle highlight
        blockquoteBorder: palette.blue, // #7aa2f7 - accent color for emphasis
    },
    spacing: defaultSpacing,
    radius: defaultRadius,
    typography: defaultTypography,
    motion: defaultMotion,
};

export const tokyoNight = tokyoNightTheme.colors;
export const tokyoNightPalette = palette;
