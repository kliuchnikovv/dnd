import { ThemeColors } from '@genie/front/themes';
import { Meter } from '../../turnview/types';

// Скин правила «Порог» — конфиг-слой, НЕ рендерер. Здесь (и только здесь, как в cli/refview.go)
// легально называются машинные kind мер, чтобы придать им цвет дизайна «Ночной детектив».
// Рендерер MeterRow остаётся агностичным: он принимает эту функцию пропом и не знает слов.
// Появится второе правило — появится второй скин, а компонент не дрогнет.

export function havenMeterTone(m: Meter, c: ThemeColors): string {
    switch (m.kind) {
        case 'harm':
            return c.wound ?? c.error ?? c.attention;
        case 'clock':
            return c.bloodAccent ?? c.accent;
        case 'grit':
        default:
            return c.accent;
    }
}
