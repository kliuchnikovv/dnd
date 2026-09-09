import { sessionKindOf, kindForResumedSession, VIGNETTE_KIND, VIGNETTE_RULES_KIND } from '../sessionKind';
import { CaseSummary } from '../../../net/cases';

test('sessionKindOf: kind="vignette" -> vignette; всё остальное -> turn', () => {
    expect(sessionKindOf({ kind: VIGNETTE_KIND, rules: 'threshold' })).toBe('vignette');
    expect(sessionKindOf({ kind: 'adventure', rules: 'dnd5e' })).toBe('turn');
    expect(sessionKindOf({ kind: 'adventure', rules: 'threshold' })).toBe('turn');
    expect(sessionKindOf(undefined)).toBe('turn');
});

test('sessionKindOf: фолбэк на rules для старого сервера без kind', () => {
    // Сервер до 2026-09-09: kind ещё пуст в ответе — читаем по rules.
    expect(sessionKindOf({ kind: '', rules: VIGNETTE_RULES_KIND })).toBe('vignette');
    expect(sessionKindOf({ kind: '', rules: 'threshold' })).toBe('turn');
    expect(sessionKindOf({ kind: '', rules: 'dnd5e' })).toBe('turn');
});

test('kindForResumedSession: joins by caseId against the loaded catalog', () => {
    const cases: CaseSummary[] = [
        { id: 'nightguest', name: 'Гость к ночи', kind: VIGNETTE_KIND, rules: VIGNETTE_RULES_KIND, scenario: 'vignette', blurb: '' },
        { id: 'harbour', name: 'Порт', kind: 'adventure', rules: 'threshold', scenario: 'deduction', blurb: '' },
    ];
    expect(kindForResumedSession('nightguest', cases)).toBe('vignette');
    expect(kindForResumedSession('harbour', cases)).toBe('turn');
});

test('kindForResumedSession: case missing from catalog falls back to turn (unproven-safe, see comment)', () => {
    expect(kindForResumedSession('deleted-case', [])).toBe('turn');
});
