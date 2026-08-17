package threshold

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
)

func TestThresholdIsClosedSet(t *testing.T) {
	cases := map[string]int{"easy": 10, "normal": 14, "hard": 18, "": 14, "нечто": 14}
	for in, want := range cases {
		if got := ThresholdFor(in); got != want {
			t.Errorf("ThresholdFor(%q) = %d, ожидалось %d", in, got, want)
		}
	}
}

func TestSituationalCountsFactorsAndClamps(t *testing.T) {
	cases := []struct {
		name string
		in   core.Intent
		view core.SceneView
		want int
	}{
		{"пусто", core.Intent{}, core.SceneView{}, 0},
		{"укрытие", core.Intent{}, core.SceneView{Cover: true}, 2},
		{"не обнаружен", core.Intent{}, core.SceneView{Undetected: true}, 2},
		{"превосходство", core.Intent{}, core.SceneView{Allies: 3, Foes: 1}, 2},
		{"меньшинство", core.Intent{}, core.SceneView{Allies: 1, Foes: 3}, -2},
		{"паритет", core.Intent{}, core.SceneView{Allies: 2, Foes: 2}, 0},
		{"темнота", core.Intent{}, core.SceneView{NodeTags: []string{"dark"}}, -2},
		{"темнота и дождь считаются один раз", core.Intent{},
			core.SceneView{NodeTags: []string{"dark", "rain"}}, -2},
		{"ранения", core.Intent{}, core.SceneView{Harm: 2}, -4},
		{"tier выше", core.Intent{}, core.SceneView{ActorTier: 2, TargetTier: 1}, 2},
		{"tier ниже", core.Intent{}, core.SceneView{ActorTier: 1, TargetTier: 2}, -2},
		{"инструмент", core.Intent{Args: core.Args{Item: "crowbar"}},
			core.SceneView{Tools: []string{"crowbar"}}, 2},
		{"инструмента нет в руках", core.Intent{Args: core.Args{Item: "crowbar"}},
			core.SceneView{}, 0},
		{"верхний кламп", core.Intent{},
			core.SceneView{Cover: true, Undetected: true, Allies: 3, Foes: 1, ActorTier: 2}, 4},
		{"нижний кламп", core.Intent{},
			core.SceneView{Harm: 3, NodeTags: []string{"dark"}, Allies: 1, Foes: 4}, -4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Situational(c.in, c.view); got != c.want {
				t.Errorf("Situational = %d, ожидалось %d", got, c.want)
			}
		})
	}
}

func TestTagAppliesByListNotBySense(t *testing.T) {
	s := Sheet{Tags: []Tag{{Name: "портовый", Verbs: []string{"question", "search"}}}}
	if got := s.TagBonus("question", core.SceneView{}); got != 2 {
		t.Errorf("тег не применился к глаголу из списка: %d", got)
	}
	if got := s.TagBonus("strike", core.SceneView{}); got != 0 {
		t.Errorf("тег применился к глаголу вне списка: %d", got)
	}
}

func TestTagAppliesByNodeTag(t *testing.T) {
	s := Sheet{Tags: []Tag{{Name: "ночной", NodeTags: []string{"dark"}}}}
	if got := s.TagBonus("strike", core.SceneView{NodeTags: []string{"dark"}}); got != 2 {
		t.Errorf("тег обстановки не применился: %d", got)
	}
	if got := s.TagBonus("strike", core.SceneView{NodeTags: []string{"crowd"}}); got != 0 {
		t.Errorf("тег обстановки применился не к той сцене: %d", got)
	}
}

func TestTagBonusNeverStacks(t *testing.T) {
	// Два подходящих тега дают +2, а не +4: бонус за теги ограничен.
	s := Sheet{Tags: []Tag{
		{Name: "портовый", Verbs: []string{"question"}},
		{Name: "дознаватель", Verbs: []string{"question"}},
	}}
	if got := s.TagBonus("question", core.SceneView{}); got != 2 {
		t.Errorf("TagBonus = %d, ожидалось 2", got)
	}
}

func TestAttrMapsVerbClassToAttribute(t *testing.T) {
	s := Sheet{Attrs: map[string]int{"body": 1, "edge": 0, "mind": 3, "will": 0}}
	if got := s.Attr("question"); got != 3 {
		t.Errorf("investigate должен читать mind: %d", got)
	}
	if got := s.Attr("strike"); got != 1 {
		t.Errorf("attack должен читать body: %d", got)
	}
	if got := s.Attr("persuade"); got != 0 {
		t.Errorf("social должен читать will: %d", got)
	}
}
