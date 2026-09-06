package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/scenarios/deduction/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

func clocksDB() *store.DB {
	db := store.NewDB()
	db.Clocks["c_suspicion"] = &store.Clock{
		ID: "c_suspicion", Name: "Подозрение", Segments: 4, TickPolicy: "on_cost",
		OnFill: store.Consequence{FlavourKey: "clock.suspicion.filled", HostileTo: []store.EntityID{"e_toke"}},
	}
	return db
}

func TestTickFillsAndFiresOnce(t *testing.T) {
	c := NewClocks(clocksDB())
	if got := c.Tick("c_suspicion", 3); len(got) != 0 {
		t.Fatalf("часы сработали раньше заполнения: %v", got)
	}
	fired := c.Tick("c_suspicion", 1)
	if len(fired) != 1 || fired[0].FlavourKey != "clock.suspicion.filled" {
		t.Fatalf("последствие не сработало на заполнении: %v", fired)
	}
	// Повторные тики переполненных часов последствие не повторяют.
	if got := c.Tick("c_suspicion", 5); len(got) != 0 {
		t.Errorf("последствие сработало повторно: %v", got)
	}
	if !c.Filled("c_suspicion") {
		t.Error("часы не отмечены как заполненные")
	}
}

func TestTickNeverExceedsSegments(t *testing.T) {
	c := NewClocks(clocksDB())
	c.Tick("c_suspicion", 99)
	snap := c.Snapshot()
	if snap[0].Filled != snap[0].Segments {
		t.Errorf("filled=%d при segments=%d — счётчик убежал", snap[0].Filled, snap[0].Segments)
	}
}

func TestTickOnUnknownClockIsNoop(t *testing.T) {
	c := NewClocks(clocksDB())
	if got := c.Tick("c_nonexistent", 1); len(got) != 0 {
		t.Errorf("тик несуществующих часов дал последствие: %v", got)
	}
}

// Висяк — тоже развязка: часы вышли, верного обвинения нет, и прогон обязан
// закончиться текстом, а не молчанием.
func TestStalledWhenEveryPressureClockIsFull(t *testing.T) {
	g := accuseGame()
	if g.Stalled() {
		t.Fatal("дело объявлено висяком на первом ходу")
	}
	g.C.Tick("c_suspicion", 6)
	if !g.Stalled() {
		t.Error("часы заполнены, а дело не висяк")
	}
}

func TestSolvedCaseIsNotStalled(t *testing.T) {
	g := accuseGame()
	learnAll(g)
	g.Accuse(accusation.Form{Who: "toke", How: "cord", When: "night", Why: "audit"})
	g.C.Tick("c_suspicion", 6)
	if g.Stalled() {
		t.Error("раскрытое дело объявлено висяком")
	}
}

// Тик от прохождения времени. Наблюдение и слежка тратят игровое время по
// самой своей природе — просидеть вечер в тени бочек стоит вечера, удачно или
// нет. Все прочие пробы занимают минуты, и время на них не списывается.
func TestTimeSpendingVerbsTickTheClock(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	before := g.C.Snapshot()[0].Filled

	g.Apply(Intent{Verb: "stake_out", Args: Args{Target: "e_toke"}})

	if got := g.C.Snapshot()[0].Filled; got != before+1 {
		t.Errorf("наблюдение не потратило времени: часы %d -> %d", before, got)
	}
}

// Успешный расспрос времени не тратит: иначе тикает вообще всё, и часы
// перестают что-либо значить.
func TestOrdinaryProbeSpendsNoTime(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	before := g.C.Snapshot()[0].Filled

	g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_open"}})

	if got := g.C.Snapshot()[0].Filled; got != before {
		t.Errorf("обычная проба потратила время: часы %d -> %d", before, got)
	}
}

// Отказ времени не тратит: ход не состоялся.
func TestRefusedObservationSpendsNoTime(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	before := g.C.Snapshot()[0].Filled

	res := g.Apply(Intent{Verb: "stake_out", Args: Args{Target: "e_nobody"}})

	if !res.Refused {
		t.Fatal("наблюдение за несуществующим принято")
	}
	if got := g.C.Snapshot()[0].Filled; got != before {
		t.Errorf("отказ потратил время: часы %d -> %d", before, got)
	}
}

// Свободная проба остаётся бесплатной всегда: игра, наказывающая за то, что
// игрок смотрит по сторонам, даст ложный негатив на весь гейт.
func TestFreeProbesNeverSpendTime(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	before := g.C.Snapshot()[0].Filled

	for _, v := range []Verb{"look", "say", "emote", "talk_to", "thank"} {
		g.Apply(Intent{Verb: v, Args: Args{Target: "e_toke", Text: "что-нибудь"}})
	}
	g.Apply(Intent{Verb: "theorize", Args: Args{Text: "Токе лжёт"}})

	if got := g.C.Snapshot()[0].Filled; got != before {
		t.Errorf("свободные пробы потратили время: часы %d -> %d", before, got)
	}
}

// Последствие заполнившихся часов применяется ровно один раз. HostileTo
// сдвигает расположение, и повторный проход по тому же списку удвоил бы
// сдвиг молча.
type tickingRules struct{}

func (tickingRules) Resolve(Intent, SceneView, Dice) Resolution {
	return Resolution{Class: OutcomePartial, Margin: -2, Costs: []CostKind{CostTickClock}}
}
func (tickingRules) PassiveScore(SceneView) int { return 10 }

func TestClockConsequenceAppliesOnce(t *testing.T) {
	// Цена провала и трата времени бьют по одним и тем же часам в одном ходу.
	g := turnGame(OutcomeSuccess)
	g.Rules = tickingRules{}
	g.DB.Clocks["c_suspicion"] = &store.Clock{
		ID: "c_suspicion", Segments: 1, TickPolicy: "on_cost",
		OnFill: store.Consequence{HostileTo: []store.EntityID{"e_toke"}},
	}
	before := g.D.Disposition("e_toke")

	g.Apply(Intent{Verb: "stake_out", Args: Args{Target: "e_toke"}})

	if got := g.D.Disposition("e_toke"); got != before-2 {
		t.Errorf("расположение %d -> %d, ожидалось -2, а не двойное применение", before, got)
	}
}
