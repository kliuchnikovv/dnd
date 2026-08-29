import * as fs from 'fs';
import * as path from 'path';

// Guard агностичности презентации (ADR-0006, аналог architecture_test ядра).
// Рендерер turn-view и экраны рисуют ДЕСКРИПТОРЫ, не конкретику: в них не должно быть
// зашитого словаря конкретного ПРАВИЛА («grit», «d20», «harm») или СЦЕНАРИЯ («casebook»,
// слоты who/how/when/why). Эти слова легальны только в конфиг-слое (mocks/haven/skin.ts,
// как cli/refview.go) и в данных-фикстурах — их этот тест не сканирует.
//
// Комментарии срезаются: документировать запрет словом внутри // … законно; нарушение — литерал
// в исполняемом коде.

const ROOT = path.resolve(__dirname, '..', '..', '..'); // src/
const SCAN_DIRS = ['components/turnview', 'screens'];

function stripComments(src: string): string {
    return src.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/\/\/[^\n]*/g, ' ');
}

function collectFiles(dir: string): string[] {
    const abs = path.join(ROOT, dir);
    if (!fs.existsSync(abs)) return [];
    const out: string[] = [];
    for (const entry of fs.readdirSync(abs, { withFileTypes: true })) {
        const p = path.join(abs, entry.name);
        if (entry.isDirectory()) {
            if (entry.name === '__tests__') continue;
            out.push(...collectFiles(path.join(dir, entry.name)));
        } else if (/\.(ts|tsx)$/.test(entry.name)) {
            out.push(p);
        }
    }
    return out;
}

const FORBIDDEN: { name: string; re: RegExp }[] = [
    { name: 'rule vocabulary (grit/d20/casebook)', re: /\b(grit|d20|casebook)\b/i },
    { name: 'rule meter kind "harm"', re: /\bharm\b/i },
    { name: 'scenario accusation slot keys who/how/when/why as literals', re: /(['"])(who|how|when|why)\1/i },
];

describe('presentation agnosticism guard', () => {
    const files = SCAN_DIRS.flatMap(collectFiles);

    it('scans at least the renderer components', () => {
        expect(files.length).toBeGreaterThan(0);
    });

    it.each(FORBIDDEN)('renderer/screens contain no hardcoded $name', ({ re }) => {
        const offenders: string[] = [];
        for (const file of files) {
            const code = stripComments(fs.readFileSync(file, 'utf8'));
            if (re.test(code)) {
                offenders.push(path.relative(ROOT, file));
            }
        }
        expect(offenders).toEqual([]);
    });
});
