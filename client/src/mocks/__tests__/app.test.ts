import { rulesets, thresholdCreation, dnd5eCreation, vignetteCreation } from '../app';

describe('rulesets', () => {
  it('содержит все три ruleset и в ожидаемом порядке', () => {
    expect(rulesets).toEqual([thresholdCreation, dnd5eCreation, vignetteCreation]);
  });

  it('каждый ruleset и каждый архетип имеют валидную форму', () => {
    for (const ruleset of rulesets) {
      expect(typeof ruleset.ruleName).toBe('string');
      expect(ruleset.ruleName.length).toBeGreaterThan(0);
      expect(ruleset.archetypes.length).toBeGreaterThan(0);

      for (const archetype of ruleset.archetypes) {
        expect(archetype.name.length).toBeGreaterThan(0);
        expect(archetype.id.length).toBeGreaterThan(0);
        expect(archetype.blurb.length).toBeGreaterThan(0);
        // Виньетка — единственный ruleset без статов (лист не читается,
        // мерка идёт по действию, ADR-0009). Для остальных проверяем.
        if (ruleset !== vignetteCreation) {
          expect(archetype.stats.length).toBeGreaterThan(0);
        }
        expect(archetype.tags.length).toBeGreaterThan(0);
        expect(archetype.token.length).toBeGreaterThan(0);
      }
    }
  });

  it('dnd5eCreation: 4 архетипа с полным набором из шести D&D-статов', () => {
    expect(dnd5eCreation.archetypes).toHaveLength(4);
    const ids = dnd5eCreation.archetypes.map((a) => a.id).sort();
    expect(ids).toEqual(['cleric', 'fighter', 'ranger', 'rogue']);

    for (const archetype of dnd5eCreation.archetypes) {
      const labels = archetype.stats.map((s) => s.label).sort();
      expect(labels).toEqual(['Cha', 'Con', 'Dex', 'Int', 'Str', 'Wis']);
    }
  });
});
