package core

import "testing"

func TestOutcomeStepsUpAndDownWithinBounds(t *testing.T) {
	cases := []struct {
		in       Outcome
		up, down Outcome
	}{
		{OutcomeFail, OutcomePartial, OutcomeFail}, // ниже провала ступени нет
		{OutcomePartial, OutcomeSuccess, OutcomeFail},
		{OutcomeSuccess, OutcomeCrit, OutcomePartial},
		{OutcomeCrit, OutcomeCrit, OutcomeSuccess}, // выше крита ступени нет
	}
	for _, c := range cases {
		if got := c.in.Up(); got != c.up {
			t.Errorf("%v.Up() = %v, ожидалось %v", c.in, got, c.up)
		}
		if got := c.in.Down(); got != c.down {
			t.Errorf("%v.Down() = %v, ожидалось %v", c.in, got, c.down)
		}
	}
}

func TestEveryCostKindHasName(t *testing.T) {
	all := AllCostKinds()
	if len(all) != 8 {
		t.Fatalf("в таксономии %d элементов, ожидалось 8", len(all))
	}
	seen := map[CostKind]bool{}
	for _, c := range all {
		if c == "" {
			t.Error("пустой элемент таксономии")
		}
		if seen[c] {
			t.Errorf("дубликат в таксономии: %q", c)
		}
		seen[c] = true
	}
}

func TestOutcomeStringIsHumanReadable(t *testing.T) {
	want := map[Outcome]string{
		OutcomeFail: "ПРОВАЛ", OutcomePartial: "ЧАСТИЧНО",
		OutcomeSuccess: "УСПЕХ", OutcomeCrit: "КРИТ",
	}
	for o, w := range want {
		if got := o.String(); got != w {
			t.Errorf("%d.String() = %q, ожидалось %q", int(o), got, w)
		}
	}
}
