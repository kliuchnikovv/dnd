import { havenViews, havenTransitions } from '../case';
import { TurnView } from '../../../turnview/types';

// Паритет с контрактом view/turnview.go проверяется в основном на компиляции (фикстуры
// типизированы как TurnView). Здесь — рантайм-инварианты, которые типы не ловят:
// обязательность bool-полей surface и непрозрачность токенов.

const views = Object.values(havenViews) as TurnView[];

describe('haven fixtures — contract invariants', () => {
    it('every meter carries an explicit boolean surface (non-omitempty in Go)', () => {
        for (const v of views) {
            for (const m of v.meters ?? []) {
                expect(typeof m.surface).toBe('boolean');
                expect(typeof m.kind).toBe('string');
                expect(m.kind.length).toBeGreaterThan(0);
            }
        }
    });

    it('every objective panel carries an explicit boolean surface', () => {
        for (const v of views) {
            if (v.objective) {
                expect(typeof v.objective.surface).toBe('boolean');
                expect(typeof v.objective.kind).toBe('string');
            }
        }
    });

    it('every option carries a non-empty opaque token', () => {
        for (const v of views) {
            for (const o of v.options ?? []) {
                expect(typeof o.token).toBe('string');
                expect(o.token.length).toBeGreaterThan(0);
            }
        }
    });

    it('every outcome carries a machine tier', () => {
        for (const v of views) {
            if (v.resolution) {
                expect(['fail', 'partial', 'success', 'crit']).toContain(v.resolution.outcome.tier);
            }
        }
    });

    it('at least one fixture surfaces a hidden meter (proves surface filtering is exercised)', () => {
        const hasHidden = views.some((v) => (v.meters ?? []).some((m) => m.surface === false));
        expect(hasHidden).toBe(true);
    });

    it('every transition target names a real view', () => {
        for (const target of Object.values(havenTransitions)) {
            expect(Object.keys(havenViews)).toContain(target);
        }
    });
});
