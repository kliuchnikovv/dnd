export type ThemeName =
    | 'nightDetective'
    | 'tokyoNight'
    | 'oneDark'
    | 'visualStudioLight'
    | 'serikaDark'
    | 'retroSunset'
    | 'retroPastel'
    | 'deepOcean'
    | 'darkMinimal';

export type ThemePalette = {
    red: string;
    orange: string;
    yellow: string;
    green: string;
    cyan: string;
    blue: string;
    magenta: string;
    purple: string; // Added for ActiveTaskCard component
    white: string;
    fg: string;
    black: string;
    bg: string;
    gutterGrey: string;
    commentGrey: string;
    selection?: string;
    background: string;
    foreground: string;
    currentLine?: string;
    comment?: string;
};

export type ThemeColors = {
    attention: string;
    background: string;
    icon: string;
    text: string;
    fadedText: string;
    accent: string;
    accentAlt: string;
    border: string;
    textSecondary: string;
    textPrimary: string;
    surface: string;
    surfaceAlt: string;
    surfaceHighlight: string;
    primary?: string;
    error?: string;
    card?: string;
    // Extended semantic colors
    warning?: string;
    success?: string;
    info?: string;
    // Source-specific colors for visual diversity
    calendar?: string;
    email?: string;
    task?: string;
    // Code colors
    codeBg?: string;
    codeInlineBg?: string;
    codeText?: string;
    // Blockquote colors
    blockquoteBg?: string;
    blockquoteBorder?: string;
    // --- Дознание («Ночной детектив») semantic extensions ---
    // Depth / surfaces beyond the base contract.
    bgDeep?: string;
    hairline?: string;
    stroke?: string;
    strokeStrong?: string;
    // Ink hierarchy (warm paper text).
    inkSecondary?: string;
    inkMuted?: string;
    inkFaint?: string;
    // Amber accent family ("one lantern").
    amberText?: string;
    amberFill?: string;
    amberHi?: string;
    // Role colors keyed to turn-view descriptors:
    //   moss  → confirmed fact (corroboration ✓)
    //   blood → refusal / accusation (Outcome.Tier "fail", counters)
    //   slate → Master voice (Block.Kind "gm")
    //   wound → harm pip / negative world event
    moss?: string;
    blood?: string;
    bloodAccent?: string;
    slate?: string;
    wound?: string;
};

export type ShadowStyle = {
    shadowColor: string;
    shadowOffset: { width: number; height: number };
    shadowOpacity: number;
    shadowRadius: number;
    elevation?: number;
};

export type ThemeEffects = {
    cardShadowRaised: ShadowStyle;
    cardShadowInset: ShadowStyle;
    cardBorderRadius: number;
    cardPadding: number;
    glassBackground: string;
    glassBorderColor: string;
    glassBorderWidth: number;
    blurIntensity: number;
    blurTint: string;
    buttonShadowRaised: ShadowStyle;
    buttonShadowPressed: ShadowStyle;
    iconBgBackground: string;
    iconBgBorderColor: string;
};

export type ThemeSpacing = {
    xs: number;
    sm: number;
    md: number;
    lg: number;
    xl: number;
    xxl: number;
};

export type ThemeRadius = {
    sm: number;
    md: number;
    lg: number;
    xl: number;
    pill: number;
};

export type TypographyVariant = {
    fontFamily: string;
    fontSize: number;
    lineHeight: number;
    fontWeight:
        | 'normal'
        | 'bold'
        | '100'
        | '200'
        | '300'
        | '400'
        | '500'
        | '600'
        | '700'
        | '800'
        | '900';
    letterSpacing?: number;
};

export type ThemeTypography = {
    display: TypographyVariant;
    title: TypographyVariant;
    body: TypographyVariant;
    caption: TypographyVariant;
    mono: TypographyVariant;
};

export type ThemeMotion = {
    fast: number;
    base: number;
    slow: number;
};

export type ThemeDescriptor = {
    id: ThemeName;
    name: string;
    variant: 'light' | 'dark';
    palette: ThemePalette;
    colors: ThemeColors;
    effects?: ThemeEffects;
    spacing: ThemeSpacing;
    radius: ThemeRadius;
    typography: ThemeTypography;
    motion: ThemeMotion;
};
