import {
    defaultMotion,
    defaultRadius,
    defaultSpacing,
    defaultTypography,
} from './tokens';
import { ThemeDescriptor, ThemePalette } from './types';

const palette: ThemePalette = {
    red: '#e06c75',
    orange: '#d19a66',
    yellow: '#e5c07b',
    green: '#98c379',
    cyan: '#56b6c2',
    blue: '#61afef',
    magenta: '#c678dd',
    purple: '#c678dd', // Same as magenta
    black: '#282c34',
    white: '#abb2bf',
    fg: '#abb2bf',
    bg: '#21252b',
    gutterGrey: '#4b5263',
    commentGrey: '#5c6370',
    // Additional colors for chat UI
    background: '#282c34',
    foreground: '#abb2bf',
    currentLine: '#2c323c',
    selection: '#3e4451',
    comment: '#5c6370',
};

export const oneDarkTheme: ThemeDescriptor = {
    id: 'oneDark',
    name: 'One Dark',
    variant: 'dark',
    palette: palette,
    colors: {
        attention: palette.red,
        background: palette.background,
        icon: palette.white,
        text: palette.white,
        fadedText: 'rgba(171, 178, 191, 0.7)',
        accent: palette.blue,
        accentAlt: palette.cyan,
        border: palette.gutterGrey,
        textSecondary: palette.commentGrey,
        textPrimary: palette.fg,
        surface: '#2c313a',
        surfaceAlt: '#262b33',
        surfaceHighlight: palette.selection ?? '#3e4451',
        primary: palette.blue,
        error: palette.red,
        card: '#2c313a',
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
        codeInlineBg: '#2c313a',
        codeText: palette.fg,
        // Blockquote colors
        blockquoteBg: '#262b33', // surfaceAlt
        blockquoteBorder: palette.blue, // accent
    },
    spacing: defaultSpacing,
    radius: defaultRadius,
    typography: defaultTypography,
    motion: defaultMotion,
};

export const oneDark = oneDarkTheme.colors;
