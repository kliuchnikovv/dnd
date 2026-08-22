package core

import "testing"

func TestVerbRegistryCoversEveryListedVerb(t *testing.T) {
	want := []Verb{
		"look", "emote", "say",
		"talk_to", "ask_about", "thank", "threaten_verbally", "theorize",
		"present",
		"examine", "search", "question", "stake_out", "tail",
		"compare", "cross_reference",
		"strike", "grapple",
		"move_zone", "take_cover", "flee",
		"sneak", "pick", "recall",
		"persuade", "intimidate", "command",
		"aid", "mend",
		"use_ability", "use_item",
	}
	if len(want) != 31 {
		t.Fatalf("список в тесте испорчен: %d глаголов вместо 31", len(want))
	}
	for _, v := range want {
		if _, ok := Verbs[v]; !ok {
			t.Errorf("глагол %q отсутствует в реестре", v)
		}
	}
	if len(Verbs) != len(want) {
		t.Errorf("в реестре %d глаголов, в списке %d — есть лишние", len(Verbs), len(want))
	}
}

func TestEveryVerbHasKnownClass(t *testing.T) {
	known := map[VerbClass]bool{
		ClassNone: true, ClassInvestigate: true, ClassReason: true,
		ClassSocial: true, ClassMove: true, ClassAttack: true,
		ClassSupport: true, ClassResource: true, ClassSkill: true,
	}
	for v, d := range Verbs {
		if !known[d.Class] {
			t.Errorf("глагол %q имеет неизвестный класс %q", v, d.Class)
		}
	}
}

func TestCompareNeverRolls(t *testing.T) {
	// Противоречие между двумя известными фактами — свойство данных, не удача.
	if Verbs["compare"].Rolls {
		t.Error("compare требует броска — это ошибка правил, а не реализации")
	}
	if !Verbs["cross_reference"].Rolls {
		t.Error("cross_reference обязан требовать броска")
	}
}

func TestFlavourAndSoftVerbsNeverRoll(t *testing.T) {
	for _, v := range []Verb{"look", "emote", "say", "talk_to", "ask_about",
		"thank", "threaten_verbally", "theorize"} {
		if Verbs[v].Rolls {
			t.Errorf("глагол %q не должен требовать броска", v)
		}
	}
}

func TestLookupVerbRejectsUnknown(t *testing.T) {
	if _, ok := LookupVerb("hack_the_gibson"); ok {
		t.Error("неизвестный глагол опознан как известный")
	}
	if d, ok := LookupVerb("question"); !ok || d.Class != ClassInvestigate {
		t.Errorf("question опознан неверно: %+v %v", d, ok)
	}
}
