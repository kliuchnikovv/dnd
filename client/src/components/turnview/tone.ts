import { ThemeColors } from '@genie/front/themes';

// Generic-лестницы вида, НЕ словарь конкретного правила/сценария. Красить по ним можно:
// у любого правила есть ступень исхода (tier), у любого присутствующего — знак расположения.
// (Meter.kind — это уже словарь ПРАВИЛА, по нему рендерер не ветвится; окраску мер даёт скин.)

export type Tone = { fg: string; fill: string };

/** Outcome.Tier → тон. Ступени "fail"|"partial"|"success"|"crit" общие для любого правила. */
export function tierTone(tier: string, c: ThemeColors): Tone {
    switch (tier) {
        case 'crit':
        case 'success':
            return { fg: c.moss ?? c.success ?? c.accent, fill: 'rgba(123,147,112,0.14)' };
        case 'partial':
            return { fg: c.amberText ?? c.accent, fill: c.amberFill ?? 'rgba(196,122,44,0.14)' };
        case 'fail':
        default:
            return { fg: c.bloodAccent ?? c.error ?? c.attention, fill: 'rgba(138,34,34,0.14)' };
    }
}

/** Who.Disposition знак → тон. Отношение общее для любого сценария. */
export function dispositionTone(disposition: number, c: ThemeColors): string {
    if (disposition > 0) return c.moss ?? c.success ?? c.accent;
    if (disposition < 0) return c.bloodAccent ?? c.error ?? c.attention;
    return c.inkMuted ?? c.textSecondary;
}
