package cyberpunk

import "testing"

func TestDeathSaveStrictLessThanBody(t *testing.T) {
	// BODY=6: roll 5 без штрафа проходит (5<6), roll 6 — нет (6==BODY).
	if !DeathSave(6, 0, 5) {
		t.Errorf("BODY=6 roll=5 penalty=0 обязан выжить (5<6)")
	}
	if DeathSave(6, 0, 6) {
		t.Errorf("BODY=6 roll=6 penalty=0 обязан умереть (не строго меньше)")
	}
}

func TestDeathSavePenaltyCumulative(t *testing.T) {
	// BODY=6, roll=3. Первый ход penalty=0 → 3<6 выжил.
	// Второй ход penalty=1 → 3+1=4<6 выжил.
	// Третий ход penalty=2 → 3+2=5<6 выжил.
	// Четвёртый ход penalty=3 → 3+3=6, НЕ строго меньше → умер.
	if !DeathSave(6, 0, 3) {
		t.Errorf("t=0: обязан выжить")
	}
	if !DeathSave(6, 1, 3) {
		t.Errorf("t=1: обязан выжить")
	}
	if !DeathSave(6, 2, 3) {
		t.Errorf("t=2: обязан выжить")
	}
	if DeathSave(6, 3, 3) {
		t.Errorf("t=3: обязан умереть (penalty докатил до BODY)")
	}
}
