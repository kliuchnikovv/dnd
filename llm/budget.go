package llm

import (
	"sync"
	"time"
)

// Caps — потолки расхода. Ноль означает «этот потолок не применяется»,
// чтобы тесты и офлайновые прогоны не приходилось обвешивать числами.
type Caps struct {
	PerUserDailyMicro  int64
	PerPartyDailyMicro int64
	GlobalDailyMicro   int64
	// PerTurnCalls — сколько вызовов допустимо на один ход. Это защита от
	// цикла регенерации: без неё цикл не ограничен ничем.
	PerTurnCalls int
	// ForecastDailyMicro — ожидаемый суточный расход. Превышение вдвое
	// вызывает Alert. Порог относительный намеренно: абсолютный порог
	// молчит на малом масштабе и вопит на большом.
	ForecastDailyMicro int64
}

// Ledger — учёт расхода. Держит суммы по дню; смена дня обнуляет счётчики.
type Ledger struct {
	mu sync.Mutex

	now   func() time.Time
	day   string
	caps  Caps
	alert func(spentMicro, forecastMicro int64)

	global    int64
	byUser    map[string]int64
	byParty   map[string]int64
	turnCalls map[string]int
	byRole    map[Role]int64
	bits      int
	alerted   bool
	killed    bool
}

type LedgerOption func(*Ledger)

// WithClock подменяет часы — тесты обязаны быть детерминированными.
func WithClock(now func() time.Time) LedgerOption {
	return func(l *Ledger) { l.now = now }
}

// WithAlert задаёт обработчик превышения прогноза вдвое.
func WithAlert(f func(spentMicro, forecastMicro int64)) LedgerOption {
	return func(l *Ledger) { l.alert = f }
}

func NewLedger(caps Caps, opts ...LedgerOption) *Ledger {
	l := &Ledger{
		now: time.Now, caps: caps,
		byUser: map[string]int64{}, byParty: map[string]int64{},
		turnCalls: map[string]int{}, byRole: map[Role]int64{},
	}
	for _, o := range opts {
		o(l)
	}
	l.day = l.today()
	return l
}

func (l *Ledger) today() string { return l.now().UTC().Format("2006-01-02") }

// rollover обнуляет счётчики при смене суток. Вызывается под мьютексом.
// Kill-switch тоже снимается: он суточный, а не вечный.
func (l *Ledger) rollover() {
	if d := l.today(); d != l.day {
		l.day = d
		l.global = 0
		l.byUser = map[string]int64{}
		l.byParty = map[string]int64{}
		l.turnCalls = map[string]int{}
		l.byRole = map[Role]int64{}
		l.bits = 0
		l.alerted = false
		l.killed = false
	}
}

// Admit проверяет потолки ДО вызова провайдера. Отказ не тратит ничего.
func (l *Ledger) Admit(r Request) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rollover()

	if l.killed {
		return ErrKillSwitch
	}
	if c := l.caps.PerTurnCalls; c > 0 && r.TurnID != "" && l.turnCalls[r.TurnID] >= c {
		return ErrTurnCalls
	}
	if c := l.caps.PerUserDailyMicro; c > 0 && r.UserID != "" && l.byUser[r.UserID] >= c {
		return ErrUserBudget
	}
	if c := l.caps.PerPartyDailyMicro; c > 0 && r.PartyID != "" && l.byParty[r.PartyID] >= c {
		return ErrPartyBudget
	}
	if c := l.caps.GlobalDailyMicro; c > 0 && l.global >= c {
		l.killed = true
		return ErrKillSwitch
	}
	return nil
}

// Record учитывает состоявшийся вызов. Вызывается после успеха провайдера:
// потраченное надо записать, даже если этим потолок перейдён — иначе
// перерасход останется невидимым.
func (l *Ledger) Record(r Request, resp Response) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rollover()

	l.global += resp.CostMicro
	l.byRole[r.Role] += resp.CostMicro
	if r.UserID != "" {
		l.byUser[r.UserID] += resp.CostMicro
	}
	if r.PartyID != "" {
		l.byParty[r.PartyID] += resp.CostMicro
	}
	if r.TurnID != "" {
		l.turnCalls[r.TurnID]++
	}
	// Бит — один вызов полного нарратора. Это единица, в которой считается
	// вся юнит-экономика, поэтому счётчик живёт здесь, а не в аналитике.
	if r.Role == RoleNarrator {
		l.bits++
	}
	if c := l.caps.GlobalDailyMicro; c > 0 && l.global >= c {
		l.killed = true
	}
	if f := l.caps.ForecastDailyMicro; f > 0 && !l.alerted && l.global >= 2*f {
		l.alerted = true
		if l.alert != nil {
			l.alert(l.global, f)
		}
	}
}

type Stats struct {
	SpentMicro      int64
	Bits            int
	ByRole          map[Role]int64
	Killed          bool
	CostPerBitMicro int64
}

func (l *Ledger) Stats() Stats {
	l.mu.Lock()
	defer l.mu.Unlock()
	byRole := make(map[Role]int64, len(l.byRole))
	for k, v := range l.byRole {
		byRole[k] = v
	}
	s := Stats{SpentMicro: l.global, Bits: l.bits, ByRole: byRole, Killed: l.killed}
	if l.bits > 0 {
		s.CostPerBitMicro = l.global / int64(l.bits)
	}
	return s
}

// Kill закрывает шлюз вручную — рубильник для оператора.
func (l *Ledger) Kill() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.killed = true
}
