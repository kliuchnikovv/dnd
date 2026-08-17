package threshold

import (
	"reflect"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
)

func TestCostTableCoversEveryClass(t *testing.T) {
	classes := []core.VerbClass{
		core.ClassInvestigate, core.ClassReason, core.ClassSocial, core.ClassMove,
		core.ClassAttack, core.ClassSupport, core.ClassResource, core.ClassSkill,
	}
	for _, cl := range classes {
		for _, out := range []core.Outcome{core.OutcomePartial, core.OutcomeFail} {
			got := costFor(cl, out, -3)
			if cl == core.ClassReason {
				if len(got) != 0 {
					t.Errorf("%s/%v: рассуждение не имеет цены, получено %v", cl, out, got)
				}
				continue
			}
			if len(got) == 0 {
				t.Errorf("%s/%v: цена не определена — дыра в таксономии", cl, out)
			}
			for _, c := range got {
				if !validCost(c) {
					t.Errorf("%s/%v: цена %q вне таксономии ядра", cl, out, c)
				}
			}
		}
	}
}

func TestSuccessAndCritCostNothing(t *testing.T) {
	for _, out := range []core.Outcome{core.OutcomeSuccess, core.OutcomeCrit} {
		if got := costFor(core.ClassInvestigate, out, 3); len(got) != 0 {
			t.Errorf("успех стоил %v", got)
		}
	}
}

func TestSpecificCostsMatchTable(t *testing.T) {
	cases := []struct {
		class core.VerbClass
		out   core.Outcome
		want  []core.CostKind
	}{
		{core.ClassInvestigate, core.OutcomePartial, []core.CostKind{core.CostTickClock}},
		{core.ClassInvestigate, core.OutcomeFail, []core.CostKind{core.CostFalseLead}},
		{core.ClassSocial, core.OutcomePartial, []core.CostKind{core.CostDebt}},
		{core.ClassSocial, core.OutcomeFail, []core.CostKind{core.CostTickClock, core.CostDispositionDown}},
		{core.ClassMove, core.OutcomePartial, []core.CostKind{core.CostPositionWorse}},
		{core.ClassMove, core.OutcomeFail, []core.CostKind{core.CostTickClock}},
		{core.ClassAttack, core.OutcomePartial, []core.CostKind{core.CostTickClock}},
		{core.ClassAttack, core.OutcomeFail, []core.CostKind{core.CostHarmSelf}},
		{core.ClassSupport, core.OutcomePartial, []core.CostKind{core.CostHalfEffect}},
		{core.ClassSupport, core.OutcomeFail, []core.CostKind{core.CostHarmSelf}},
		{core.ClassResource, core.OutcomePartial, []core.CostKind{core.CostResourceSpent}},
		{core.ClassResource, core.OutcomeFail, []core.CostKind{core.CostResourceSpent}},
		{core.ClassSkill, core.OutcomePartial, []core.CostKind{core.CostPositionWorse}},
		{core.ClassSkill, core.OutcomeFail, []core.CostKind{core.CostTickClock, core.CostPositionWorse}},
	}
	for _, c := range cases {
		got := costFor(c.class, c.out, -3)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s/%v: %v, ожидалось %v", c.class, c.out, got, c.want)
		}
	}
}

func TestCatastrophicMarginDoublesCost(t *testing.T) {
	normal := costFor(core.ClassSocial, core.OutcomeFail, -9)
	doubled := costFor(core.ClassSocial, core.OutcomeFail, -10)
	if len(doubled) != 2*len(normal) {
		t.Fatalf("маржа -10 дала %d элементов, ожидалось %d", len(doubled), 2*len(normal))
	}
	if !reflect.DeepEqual(doubled[:len(normal)], normal) {
		t.Error("удвоение исказило состав цены")
	}
	if !reflect.DeepEqual(doubled[len(normal):], normal) {
		t.Error("вторая половина удвоенной цены не совпадает с первой")
	}
}

func TestReasonNeverCostsEvenOnCatastrophe(t *testing.T) {
	if got := costFor(core.ClassReason, core.OutcomeFail, -20); len(got) != 0 {
		t.Errorf("рассуждение стоило %v", got)
	}
}

func validCost(c core.CostKind) bool {
	for _, k := range core.AllCostKinds() {
		if k == c {
			return true
		}
	}
	return false
}
