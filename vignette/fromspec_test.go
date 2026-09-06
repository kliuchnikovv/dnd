package vignette

import (
	"testing"

	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/scenegen"
)

// Порт прото TestSpecValidAndPlayable: сгенерированный спек после repair+validate
// маппится в играбельную сцену и доигрывается до победы. Полный конвейер
// генератора → движка (без LLM: спек берём готовый).
func TestSpecMapsToPlayableScene(t *testing.T) {
	s := &scenegen.SceneSpec{
		Title: "Часовня", Intro: "Пустая часовня, снаружи скребётся.",
		Truth: "За дверью нечто. Не войдёт без приглашения. Достоять до утра.",
		Mode:  "hold", Ambient: "Ночь.", SafeNote: "Ты у алтаря.",
		WinText: "Светает. Ты выстоял.", LoseText: "Ты отпираешь — оно входит.",
		Goal: 3, Beats: []string{"Просит впустить.", "Грозит."},
		Objects: []scenegen.ObjectSpec{
			{ID: "door", Name: "дверь", Surface: "Дверь, в неё скребутся.", Aspect: "listen",
				Tiers: []scenegen.TierSpec{{Text: "Скрёбот как коготь.", Passive: 11}, {Text: "Зовёт по имени.", Grants: "wrong"}}},
			{ID: "candle", Name: "свечи", Surface: "Свечи у алтаря.", Aspect: "look",
				Tiers: []scenegen.TierSpec{{Text: "Пламя гнётся к двери.", Passive: 11}}},
		},
	}
	scenegen.Repair(s)
	if errs := scenegen.Validate(s); len(errs) > 0 {
		t.Fatalf("спек не прошёл валидацию: %v", errs)
	}

	sc := FromSpec(s)
	if len(sc.Order) != 2 || sc.Objects["door"] == nil {
		t.Fatalf("маппинг сломан: order=%v", sc.Order)
	}
	if sc.Truth == "" {
		t.Fatal("правда потерялась при маппинге (должна уйти в сцену, не Мастеру)")
	}

	// hold доигрывается до победы за Goal ходов.
	st := NewState(dice.Fixed(10))
	var res Result
	for i := 0; i < sc.Goal; i++ {
		st.Turn++
		res = st.Adjudicate(sc, Ruling{Kind: "idle"})
	}
	if !res.Ended || res.EndText != sc.WinText {
		t.Fatalf("сгенерированный hold не завершился победой: %+v", res)
	}
}
