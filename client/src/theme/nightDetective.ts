import { defaultMotion, defaultRadius, defaultSpacing } from './tokens';
import { ThemeDescriptor, ThemeEffects, ThemePalette, ThemeTypography } from './types';

// «Ночной детектив» — направление, зафиксированное в docs/design_handoff_dnd_mobile/README.md.
// Тёмное поле, единственный тёплый акцент (амбер), три голоса шрифта (PT Serif / IBM Plex Mono / PT Sans).
// Тема слотится в общий контракт nomi ThemeDescriptor рядом с darkMinimal.

// Font-family identifiers must match the names loaded via @expo-google-fonts (see theme/fonts.ts).
export const fontFamilies = {
    serif: 'PTSerif_400Regular',
    serifItalic: 'PTSerif_400Regular_Italic',
    serifBold: 'PTSerif_700Bold',
    serifBoldItalic: 'PTSerif_700Bold_Italic',
    mono: 'IBMPlexMono_400Regular',
    monoMed: 'IBMPlexMono_500Medium',
    monoBold: 'IBMPlexMono_700Bold',
    sans: 'PTSans_400Regular',
    sansBold: 'PTSans_700Bold',
} as const;

const palette: ThemePalette = {
    red: '#E95851',
    orange: '#C47A2C',
    yellow: '#DDA060',
    green: '#7B9370',
    cyan: '#94AEB6',
    blue: '#94AEB6',
    magenta: '#C47A2C',
    purple: '#C47A2C',
    white: '#F5EFDD',
    fg: '#F5EFDD',
    black: '#0F0D0A',
    bg: '#0F0D0A',
    gutterGrey: '#221B12',
    commentGrey: 'rgba(245,239,221,0.5)',
    selection: '#221B12',
    background: '#0F0D0A',
    foreground: '#F5EFDD',
    currentLine: '#17130D',
    comment: 'rgba(245,239,221,0.5)',
};

const effects: ThemeEffects = {
    cardShadowRaised: {
        shadowColor: '#000000',
        shadowOffset: { width: 0, height: 12 },
        shadowOpacity: 0.8,
        shadowRadius: 18,
        elevation: 8,
    },
    cardShadowInset: {
        shadowColor: '#000000',
        shadowOffset: { width: 0, height: 2 },
        shadowOpacity: 0.5,
        shadowRadius: 6,
    },
    cardBorderRadius: 16,
    cardPadding: 14,
    glassBackground: 'rgba(23,19,13,0.72)',
    glassBorderColor: 'rgba(196,122,44,0.20)',
    glassBorderWidth: 1,
    blurIntensity: 24,
    blurTint: 'dark',
    buttonShadowRaised: {
        shadowColor: 'rgba(196,122,44,0.6)',
        shadowOffset: { width: 0, height: 8 },
        shadowOpacity: 0.6,
        shadowRadius: 14,
        elevation: 6,
    },
    buttonShadowPressed: {
        shadowColor: '#000000',
        shadowOffset: { width: 0, height: 2 },
        shadowOpacity: 0.4,
        shadowRadius: 4,
    },
    iconBgBackground: 'rgba(196,122,44,0.14)',
    iconBgBorderColor: 'rgba(196,122,44,0.24)',
};

// Три голоса. Размеры — из README (дизайн при 360pt): проза серифом, «терминальное» — моно.
const typography: ThemeTypography = {
    display: { fontFamily: fontFamilies.serifBold, fontSize: 22, lineHeight: 28, fontWeight: '700' },
    title: { fontFamily: fontFamilies.serifBold, fontSize: 17, lineHeight: 23, fontWeight: '700' },
    body: { fontFamily: fontFamilies.serif, fontSize: 15, lineHeight: 24, fontWeight: '400' },
    caption: { fontFamily: fontFamilies.mono, fontSize: 11, lineHeight: 16, fontWeight: '400', letterSpacing: 0.6 },
    mono: { fontFamily: fontFamilies.monoBold, fontSize: 10.5, lineHeight: 15, fontWeight: '700', letterSpacing: 1.0 },
};

export const nightDetectiveTheme: ThemeDescriptor = {
    id: 'nightDetective',
    name: 'Ночной детектив',
    variant: 'dark',
    palette,
    colors: {
        attention: '#E95851',
        background: '#0F0D0A',
        icon: '#E6C88A',
        text: '#F5EFDD',
        fadedText: 'rgba(245,239,221,0.5)',
        accent: '#C47A2C',
        accentAlt: '#DDA060',
        border: '#2A2317',
        textSecondary: 'rgba(245,239,221,0.85)',
        textPrimary: '#F5EFDD',
        surface: '#17130D',
        surfaceAlt: '#141019',
        surfaceHighlight: '#221B12',
        primary: '#C47A2C',
        error: '#E95851',
        card: '#17130D',
        warning: '#C47A2C',
        success: '#7B9370',
        info: '#94AEB6',
        codeBg: '#141019',
        codeInlineBg: '#17130D',
        codeText: '#F5EFDD',
        blockquoteBg: '#141019',
        blockquoteBorder: '#C47A2C',
        // Дознание semantic extensions
        bgDeep: '#0a0908',
        hairline: '#221B12',
        stroke: '#2A2317',
        strokeStrong: '#3B3020',
        inkSecondary: 'rgba(245,239,221,0.85)',
        inkMuted: 'rgba(245,239,221,0.55)',
        inkFaint: 'rgba(245,239,221,0.38)',
        amberText: '#DDA060',
        amberFill: 'rgba(196,122,44,0.18)',
        amberHi: '#F5D9A8',
        moss: '#7B9370',
        blood: '#8A2222',
        bloodAccent: '#E95851',
        slate: '#94AEB6',
        wound: '#8A5A4A',
    },
    effects,
    spacing: defaultSpacing,
    radius: { sm: 6, md: 12, lg: 16, xl: 22, pill: 999 },
    typography,
    motion: defaultMotion,
};

export const nightDetective = nightDetectiveTheme.colors;
export const nightDetectivePalette = palette;
