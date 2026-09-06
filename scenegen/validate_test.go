package scenegen

import "testing"

// Портированы контракты валидатора/починки прототипа (proto/scenegen_test.go):
// генератор-LLM даёт битые сцены, и валидатор — обязательный барьер. Починка
// мягко чинит пороги и гарантирует достижимый passive-tell; валидатор жёстко
// ловит структурные нарушения (ретрай ≤3 — уже на стороне вызывающего LLM).

func validHoldSpec() *SceneSpec {
	return &SceneSpec{
		Title: "Часовня на отшибе", Intro: "Ты в пустой часовне, снаружи скребётся.",
		Truth: "За дверью не паломник, а нечто. Не войдёт без приглашения. Достоять до утра.",
		Mode:  "hold", Ambient: "Ночь, свечи оплывают.", SafeNote: "Ты у алтаря, дверь заперта.",
		WinText: "Светает, скрёбот стихает. Ты выстоял.", LoseText: "Ты отпираешь — и оно входит.",
		Goal: 3, Beats: []string{"Голос просит впустить.", "Голос грозит."},
		Objects: []ObjectSpec{
			{ID: "door", Name: "дверь", Surface: "Тяжёлая дверь, в неё скребутся.", Aspect: "listen",
				Tiers: []TierSpec{{Text: "Скрёбот не как рукой — как когтем.", Passive: 11}, {Text: "Оно зовёт тебя по имени.", Passive: 0, Grants: "wrong"}}},
			{ID: "candle", Name: "свечи", Surface: "Свечи оплывают у алтаря.", Aspect: "look",
				Tiers: []TierSpec{{Text: "Пламя гнётся к двери, будто тянется сквозняк, которого нет.", Passive: 11}}},
		},
	}
}

func TestValidSpecPasses(t *testing.T) {
	s := validHoldSpec()
	Repair(s)
	if errs := Validate(s); len(errs) > 0 {
		t.Fatalf("валидная сцена дала ошибки: %v", errs)
	}
}

func TestValidatorCatchesBroken(t *testing.T) {
	// disable без win_target и без hazard — обе ошибки должны всплыть.
	s := &SceneSpec{
		Title: "x", Intro: "x", Truth: "x", Ambient: "x", SafeNote: "x", WinText: "x",
		Mode: "disable",
		Objects: []ObjectSpec{
			{ID: "a", Surface: "s", Aspect: "look", Tiers: []TierSpec{{Text: "t", Passive: 11}}},
			{ID: "b", Surface: "s", Aspect: "look", Tiers: []TierSpec{{Text: "t", Passive: 11}}},
		},
	}
	Repair(s)
	if errs := Validate(s); len(errs) == 0 {
		t.Fatal("сломанная disable-сцена прошла валидацию")
	}
}

func TestRepairEnsuresReachableTell(t *testing.T) {
	// Все тиры недостижимы/активны — починка обязана дать пассивный tell ≤12.
	s := &SceneSpec{
		Objects: []ObjectSpec{
			{ID: "a", Surface: "s", Aspect: "look", Tiers: []TierSpec{{Text: "t", Passive: 99}, {Text: "u", Passive: 0}}},
		},
	}
	Repair(s)
	reachable := false
	for _, tr := range s.Objects[0].Tiers {
		if tr.Passive > 0 && tr.Passive <= 12 {
			reachable = true
		}
	}
	if !reachable {
		t.Fatalf("починка не обеспечила достижимый tell: %+v", s.Objects[0].Tiers)
	}
}

func TestRepairClampsAndDefaultsAspect(t *testing.T) {
	s := &SceneSpec{
		Objects: []ObjectSpec{
			{ID: "a", Surface: "s", Aspect: "СТРАННОЕ", Tiers: []TierSpec{{Text: "t", Passive: -3}}},
		},
	}
	Repair(s)
	o := s.Objects[0]
	if o.Aspect != "look" {
		t.Errorf("неизвестный аспект не приведён к look: %q", o.Aspect)
	}
	if o.Tiers[0].Passive < 0 {
		t.Errorf("отрицательный порог не поднят до 0: %d", o.Tiers[0].Passive)
	}
}
