import {
    defaultMotion,
    defaultRadius,
    defaultSpacing,
    defaultTypography,
} from './tokens';
import { ThemeDescriptor, ThemePalette } from './types';

const palette: ThemePalette = {
    red: '#ca4754',
    orange: '#d28e5d',
    yellow: '#e2b714',
    green: '#98c379',
    cyan: '#56b6c2',
    blue: '#61afef',
    magenta: '#c678dd',
    purple: '#c678dd', // Same as magenta
    black: '#1e2023',
    white: '#d1d0c5',
    fg: '#d1d0c5',
    bg: '#323437',
    gutterGrey: '#4b5263',
    commentGrey: '#646669',
    // Additional colors for chat UI
    background: '#323437',
    foreground: '#d1d0c5',
    currentLine: '#3b3e42',
    selection: '#4e5157',
    comment: '#646669',
};

export const serikaDarkTheme: ThemeDescriptor = {
    id: 'serikaDark',
    name: 'Serika Dark',
    variant: 'dark',
    palette: palette,
    colors: {
        attention: palette.red,
        background: palette.background,
        icon: palette.white,
        text: palette.white,
        fadedText: 'rgba(209, 208, 197, 0.7)',
        accent: palette.yellow,
        accentAlt: palette.orange,
        border: palette.gutterGrey,
        textSecondary: palette.commentGrey,
        textPrimary: palette.fg,
        surface: '#3b3e42',
        surfaceAlt: '#2c2e31',
        surfaceHighlight: palette.selection ?? '#4e5157',
        primary: palette.yellow,
        error: palette.red,
        card: '#3b3e42',
        // Extended semantic colors
        warning: palette.orange,
        success: palette.green,
        info: palette.cyan,
        // Source-specific colors
        calendar: palette.blue,
        email: palette.red,
        task: palette.green,
        // Code colors
        codeBg: palette.currentLine,
        codeInlineBg: '#3b3e42',
        codeText: palette.fg,
        // Blockquote colors
        blockquoteBg: '#2c2e31', // surfaceAlt
        blockquoteBorder: palette.yellow, // accent
    },
    spacing: defaultSpacing,
    radius: defaultRadius,
    typography: defaultTypography,
    motion: defaultMotion,
};

export const serikaDark = serikaDarkTheme.colors;
