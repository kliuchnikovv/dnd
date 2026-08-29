import { oneDarkTheme } from './oneDark';
import { retroSunsetTheme } from './retroSunset';
import { serikaDarkTheme } from './serikaDark';
import { tokyoNightTheme } from './tokyoNight';
import { visualStudioLightTheme } from './visualStudioLight';
import { retroPastelTheme } from './retroPastel';
import { deepOceanTheme } from './deepOcean';
import { darkMinimalTheme } from './darkMinimal';
import { nightDetectiveTheme } from './nightDetective';
import { ThemeDescriptor, ThemeName } from './types';

export const themes: Record<ThemeName, ThemeDescriptor> = {
    nightDetective: nightDetectiveTheme,
    tokyoNight: tokyoNightTheme,
    oneDark: oneDarkTheme,
    visualStudioLight: visualStudioLightTheme,
    serikaDark: serikaDarkTheme,
    retroSunset: retroSunsetTheme,
    retroPastel: retroPastelTheme,
    deepOcean: deepOceanTheme,
    darkMinimal: darkMinimalTheme,
};

export const defaultThemeName: ThemeName = 'nightDetective';

export const activeTheme = themes[defaultThemeName];
export const theme = activeTheme.colors;
export const palette = activeTheme.palette;

export type { ThemeColors, ThemeDescriptor, ThemeEffects, ThemeName, ThemePalette } from './types';
