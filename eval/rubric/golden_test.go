package rubric

// Тесты золотой катки «Тихий Дол». Проверяем, что данные Эталона разложены
// корректно и что рубрика на них себя ведёт ожидаемо:
//
//   - тайна не считается утечкой, пока не заслужена (D13 на ходах 1–3);
//   - на ходе 4, где тайна заслужена, её раскрытие — легально;
//   - «тряпка» (скрытый бросок) и «вахтёр» (отбитая заявка) ловятся тем же
//     чек-листом, что и на живых прогонах (это требование Эталона §«Как
//     ловятся тряпка и вахтёр»).

import (
	"context"
	"strings"
	"testing"
)

func TestGoldenRunShape(t *testing.T) {
	turns := GoldenRun()
	if len(turns) != 9 {
		t.Fatalf("в катке %d ходов, ожидалось 9 (Эталон п.1–9)", len(turns))
	}
	for i, turn := range turns {
		if strings.TrimSpace(turn.Frame) == "" {
			t.Errorf("ход %d: пустая рамка — Мастеру нечего описывать", i+1)
		}
		if strings.TrimSpace(turn.Note) == "" {
			t.Errorf("ход %d: нет человеческой пометки из Эталона", i+1)
		}
	}
	// Старт — без заявки игрока (Мастер сам ставит сцену).
	if turns[0].Input != "" {
		t.Errorf("ход 1 должен быть стартом без заявки, got %q", turns[0].Input)
	}
	// Кости — те же, что в человеческой катке (Эталон п.3–9):
	//   п.3  16 против 14 (+2)      п.4  Трудно 18, УСПЕХ, мир: (оно: 5)
	//   п.5  тариф −2, маржа −4      п.6  нат-1, 5 против 14
	//   п.7  Легко 10, +5            п.8  нат-1; мир: d20=20 (крит)
	//   п.9  19+2-4+2 против 10 (+9)
	type roll struct {
		d20, mod, dc, total, margin int
	}
	want := []roll{
		{0, 0, 0, 0, 0}, // старт, без броска
		{0, 0, 0, 0, 0}, // разговор, без броска
		{16, 0, 14, 16, 2},
		{16, 2, 18, 18, 0},
		{16, -2, 18, 14, -4},
		{1, 4, 14, 5, -9},
		{15, 0, 10, 15, 5},
		{1, 4, 14, 5, -9},
		{19, 0, 10, 19, 9},
	}
	for i, w := range want {
		tn := turns[i]
		if w.d20 == 0 {
			if tn.Roll != "" {
				t.Errorf("ход %d: ожидался ход без броска, got %q", i+1, tn.Roll)
			}
			continue
		}
		if tn.RollD20 != w.d20 || tn.RollMod != w.mod || tn.RollDC != w.dc ||
			tn.RollTotal != w.total || tn.RollMargin != w.margin {
			t.Errorf("ход %d: бросок d20=%d mod=%d dc=%d total=%d margin=%d, ожидалось %d/%d/%d/%d/%d",
				i+1, tn.RollD20, tn.RollMod, tn.RollDC, tn.RollTotal, tn.RollMargin,
				w.d20, w.mod, w.dc, w.total, w.margin)
		}
	}
}

// Арифметика броска должна сходиться: total = d20 + mod, margin = total − dc.
// Именно на этом ловится ошибка вида «12-2 = 10 против 18» с маржой −4:
// арифметика даёт −8, и проза «мимо на 4» врёт.
func TestGoldenArithmeticConsistent(t *testing.T) {
	turns := GoldenRun()
	for i, tn := range turns {
		if tn.Roll == "" {
			continue
		}
		if tn.RollTotal != tn.RollD20+tn.RollMod {
			t.Errorf("ход %d: total %d ≠ d20 %d + mod %d",
				i+1, tn.RollTotal, tn.RollD20, tn.RollMod)
		}
		if tn.RollMargin != tn.RollTotal-tn.RollDC {
			t.Errorf("ход %d: margin %d ≠ total %d − dc %d",
				i+1, tn.RollMargin, tn.RollTotal, tn.RollDC)
		}
	}
}

// Итог хода 4 — УСПЕХ (Эталон: «Успех = сет-пис»), и мир бросал за тварь (оно: 5).
// Ошибка здесь меняла всё: A4 на ходе 4 неприменим, а C10 должен оцениваться.
func TestGoldenTurn4IsSuccessWithWorldRoll(t *testing.T) {
	turns := GoldenRun()
	tn := turns[3]
	if tn.RollMargin < 0 {
		t.Errorf("ход 4 должен быть успехом, маржа %d", tn.RollMargin)
	}
	if tn.RollDC != 18 {
		t.Errorf("ход 4: порог из Эталона — Трудно 18, got %d", tn.RollDC)
	}
	if tn.NpcRoll == nil {
		t.Fatal("ход 4: должен быть бросок мира (оно: 5) — из него тег C10")
	}
	if tn.NpcRoll.D20 != 5 {
		t.Errorf("ход 4: мир должен бросить 5, got %d", tn.NpcRoll.D20)
	}
}

// Нат-1 на ходах 6 и 8, нат-20 у мира на ходе 8.
func TestGoldenRunCriticals(t *testing.T) {
	turns := GoldenRun()
	if turns[5].RollNatural != 1 {
		t.Errorf("ход 6 должен быть нат-1, got natural=%d", turns[5].RollNatural)
	}
	if turns[7].RollNatural != 1 {
		t.Errorf("ход 8 должен быть нат-1, got natural=%d", turns[7].RollNatural)
	}
	if turns[7].NpcRoll == nil {
		t.Fatal("ход 8: должен быть бросок мира за тварь (C10)")
	}
	if turns[7].NpcRoll.D20 != 20 {
		t.Errorf("ход 8: мир должен бросить нат-20, got %d", turns[7].NpcRoll.D20)
	}
}

// D13: тайна не считается утечкой, пока не заслужена.
func TestGoldenSecretEarnedAtTurn4(t *testing.T) {
	turns := GoldenRun()
	secret := GoldenSecret

	// Ходы 1–3: тайны в раскрытом нет.
	for i := 0; i < 3; i++ {
		for _, r := range turns[i].Revealed {
			if strings.Contains(strings.ToUpper(r), secret) {
				t.Errorf("ход %d: тайна раскрыта раньше срока (%q)", i+1, r)
			}
		}
	}
	// Ход 4: тайна заслужена.
	found := false
	for _, r := range turns[3].Revealed {
		if strings.Contains(strings.ToUpper(r), secret) {
			found = true
		}
	}
	if !found {
		t.Errorf("ход 4: тайна должна быть в раскрытом")
	}
}

// D13 на реальных записях: утечка до заслуги ловится, заслуженное — нет.
func TestGoldenD13LeakDetection(t *testing.T) {
	turns := GoldenRun()
	rub := NewRubric()

	// Ход 2 (тайна не заслужена): Мастер вставил разгадку в прозу.
	leak := "Ты понимаешь: тварь привязана к твоим действиям, и потому канат подался."
	rec := turns[1].ToRecord(leak)
	if v := rub.Judge(context.Background(), D13, rec); v.Outcome != OutcomeFail {
		t.Errorf("D13 должен ловить утечку до заслуги, got %v reason=%q", v.Outcome, v.Reason)
	}

	// Тот же ход, но без утечки — pass.
	clean := turns[1].ToRecord("Мужик смотрит на тебя и говорит: после вечерника из трёх домов не вернулись.")
	if v := rub.Judge(context.Background(), D13, clean); v.Outcome != OutcomePass {
		t.Errorf("D13 ложно сработал на чистой прозе: %v reason=%q", v.Outcome, v.Reason)
	}

	// Ход 4: тайна заслужена, её раскрытие — не утечка.
	earned := turns[3].ToRecord("Ты видишь: тварь привязана к твоим действиям — дёрнул канат, и оно подалось.")
	if v := rub.Judge(context.Background(), D13, earned); v.Outcome != OutcomePass {
		t.Errorf("D13 ложно сработал на заслуженном раскрытии: %v reason=%q", v.Outcome, v.Reason)
	}
}

// Тряпка: проза без броска на ходе с броском → A2 fail.
func TestGoldenCatchesCloth(t *testing.T) {
	turns := GoldenRun()
	rub := NewRubric()
	// Ход 3 (бросок 16/14) — Мастер не показал кость.
	rec := turns[2].ToRecord("Ты бежишь к башне. Следы обрываются, пахнет осиной бумагой.")
	if v := rub.Judge(context.Background(), A2, rec); v.Outcome != OutcomeFail {
		t.Errorf("тряпка (скрытый бросок) должна ловиться A2, got %v", v.Outcome)
	}
}

// Вахтёр: Мастер отбил заявку → A1 fail (это LLM-пункт, поэтому здесь
// проверяем только то, что он ВООБЩЕ применим и требует LLM-судьи, а не
// структуры).
func TestGoldenA1IsLLMOnly(t *testing.T) {
	if _, ok := specFor(A1); !ok {
		t.Error("A1 должен иметь LLM-спеку")
	}
	turns := GoldenRun()
	rub := NewRubric()
	rec := turns[4].ToRecord("Так нельзя. Выбери: звонить или смотреть.")
	v := rub.Judge(context.Background(), A1, rec)
	if v.Outcome != OutcomeNA {
		t.Errorf("A1 без LLM-судьи должен быть NA, got %v", v.Outcome)
	}
}

// Prompt-слой: Мастеру уходит рамка + канон, и НЕ уходит ключ-ответа.
func TestGoldenPromptHidesSecret(t *testing.T) {
	turns := GoldenRun()
	frame, canon := GoldenPrompt(turns, 1)
	if frame == "" {
		t.Fatal("пустая рамка на ходе 2")
	}
	if len(canon) == 0 {
		t.Fatal("на ходе 2 должен быть канон (что игрок уже знает)")
	}
	// На ходе 2 канон пуст (первый ход ещё не заслужил знаний) — проверяем
	// на ходе 3, где знание уже есть.
	_, canon3 := GoldenPrompt(turns, 2)
	if len(canon3) == 0 {
		t.Error("на ходе 3 канон должен быть непустым")
	}
	// Ключ-ответа в рамке быть не должно: на ходе 1–3 он не заслужен.
	for i := 0; i < 3; i++ {
		f, _ := GoldenPrompt(turns, i)
		if strings.Contains(strings.ToUpper(f), GoldenSecret) {
			t.Errorf("ход %d: ключ-ответа не должно быть в рамке для Мастера", i+1)
		}
	}
}

// Секрет не должен попадать в рамку и раньше — но на ходе 4 он ЕСТЬ в рамке
// (это законное раскрытие, и Мастер должен его произнести).
func TestGoldenPromptSecretAfterEarned(t *testing.T) {
	turns := GoldenRun()
	f, _ := GoldenPrompt(turns, 3)
	if !strings.Contains(strings.ToUpper(f), GoldenSecret) {
		t.Error("ход 4: заслуженная тайна должна быть в рамке — иначе Мастер не сможет её раскрыть")
	}
}

// C10: бросок мира виден игроку.
func TestGoldenC10WorldRollVisible(t *testing.T) {
	turns := GoldenRun()
	rub := NewRubric()

	// Ход 8: мир бросил нат-20, и это сказано игроку.
	shown := "бросок: d20=1+0 = 1 против 14 · -9. Мир бросил за тварь: d20=20 — крит. Второе ранение."
	rec := turns[7].ToRecord(shown)
	if v := rub.Judge(context.Background(), C10, rec); v.Outcome != OutcomePass {
		t.Errorf("C10 pass ожидался (бросок мира виден), got %v reason=%q", v.Outcome, v.Reason)
	}

	// Тот же ход, но бросок мира спрятан — провал.
	hidden := "бросок: d20=1+0 = 1 против 14 · -9. Тварь вдруг бьёт — второе ранение."
	rec2 := turns[7].ToRecord(hidden)
	if v := rub.Judge(context.Background(), C10, rec2); v.Outcome != OutcomeFail {
		t.Errorf("C10 fail ожидался (бросок мира спрятан), got %v reason=%q", v.Outcome, v.Reason)
	}

	// Ход без броска мира — пункт неприменим.
	rec3 := turns[2].ToRecord("бросок: d20=16+0 = 16 против 14 · +2. Следы обрываются.")
	if v := rub.Judge(context.Background(), C10, rec3); v.Outcome != OutcomeNA {
		t.Errorf("C10 NA ожидался (мир не бросал), got %v reason=%q", v.Outcome, v.Reason)
	}
}

// Применимость: различение «не применим» и «нужен судья».
func TestApplicabilityDistinguishes(t *testing.T) {
	turns := GoldenRun()
	rec := turns[2].ToRecord("бросок: d20=16+0 = 16 против 14 · +2.")

	if a := ApplicabilityOf(A2, rec); a != Applies {
		t.Errorf("A2 применим на ходе с броском, got %v", a)
	}
	if a := ApplicabilityOf(A1, rec); a != NeedsJudge {
		t.Errorf("A1 должен быть NeedsJudge, got %v", a)
	}
	if a := ApplicabilityOf(D13, rec); a != Applies {
		t.Errorf("D13 применим (есть ключ-ответа), got %v", a)
	}
	// Ход без броска: A2 неприменим.
	noRoll := turns[1].ToRecord("мужик говорит.")
	if a := ApplicabilityOf(A2, noRoll); a != NotApplicable {
		t.Errorf("A2 без броска должен быть NotApplicable, got %v", a)
	}
	// Без ключ-ответа D13 неприменим.
	noTruth := rec
	noTruth.SceneTruth = ""
	if a := ApplicabilityOf(D13, noTruth); a != NotApplicable {
		t.Errorf("D13 без ключа-ответа должен быть NotApplicable, got %v", a)
	}
}
