// Package vignette — движок хоррор-виньетки: свой State/Scene/turn-loop (порт
// проверенного прототипа) и своя каноничная DC-шкала (см. dc.go); scenegen —
// для генерации сцен. Самодостаточен (ADR-0009): из ядра берётся только честная
// кость core.Dice, измерительный M1a-рулсет rules/threshold НЕ импортируется.
// Отдельный движок, а не core.Scenario: у виньетки своё состояние (Progress/
// OffPath/MireDepth/hold-turn/Beats) и свой ход (move_off/climb/bandwidth-reveal),
// которые не ложатся в M1a-ядро (FactHolder/Knowledge/turn). M1a не трогается.
//
// Анти-утечка (ADR-0008) сохранена конструкцией: правда сцены (Scene.Truth) живёт
// здесь и у стража, Мастеру уходят только revealed-строки.
package vignette

// Аспект объекта — упорядоченные ТИРЫ (shallow→deep). Сколько тиров откроется —
// решает бросок (bandwidth). Часть тиров закрыта гейтом действия/знания (правда
// открывается последствием, не даром). Пассивные тиры открываются вниманием без
// кости.

type GateKind int

const (
	GateOpen GateKind = iota
	GateCheck
	GateAction
	GateKnowledge
)

type Gate struct {
	Kind GateKind
	Key  string
}

type Tier struct {
	Gate   Gate
	Text   string
	Grants string
	// Passive: 0 — активный тир (по броску, bandwidth); >0 — пассивный порог
	// внимания (10+edge±5). Пассивный открывается детерминированно при осмотре,
	// без кости и без лока — ключевой tell не теряется с одного плохого броска.
	Passive int
}

type Aspect struct {
	Tiers []Tier
}

type Object struct {
	ID      string
	Name    string
	Surface string
	Hidden  bool
	Aspects map[string]Aspect
}

// Hazard — смертельная зона (топь/жернова). nil, если её в сцене нет.
type Hazard struct {
	EnterText string
	Depths    []string // положение по глубине 1..N
	LoseText  string
}

// Mode — режим победы: traverse (пройти Goal шагов), disable (обезвредить
// WinTarget снаряжением), hold (достоять Goal ходов, не совершив рокового шага).
const (
	ModeTraverse = "traverse"
	ModeDisable  = "disable"
	ModeHold     = "hold"
)

type Scene struct {
	Title, Intro, Truth string
	Order               []string
	Objects             map[string]*Object

	Mode      string
	Goal      int
	WinTarget string
	SafeNote  string
	Ambient   string
	WinText   string
	LoseText  string
	Hazard    *Hazard

	// Beats — расписание эскалации для hold: по строке на ход, ядро пушит
	// текущий Мастеру как СОБЫТИЕ (без проверки), давление растёт само.
	Beats []string
}

func allObjectIDs(sc *Scene) []string {
	ids := make([]string, 0, len(sc.Objects))
	seen := map[string]bool{}
	for _, id := range sc.Order {
		ids = append(ids, id)
		seen[id] = true
	}
	for id := range sc.Objects {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	return ids
}
