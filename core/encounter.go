package core

import "github.com/kliuchnikovv/dnd/store"

// Encounter — состояние боя. nil у Game — бой не идёт (обычный ход).
// Порядок ходов задаётся один раз при входе в бой (initiative от правил);
// внутри — монотонное продвижение с обёрткой на len(Order).
type Encounter struct {
	Order   []store.EntityID
	Turn    int  // индекс в Order, чей сейчас ход
	Round   int  // сквозной номер раунда
	Actions ActionState
}

// ActionState — что доступно этому существу на его такте.
// false — ещё не потрачено. reaction в первом заходе не моделируем.
type ActionState struct {
	Action bool
	Bonus  bool
}

// Current — чей сейчас такт. Пустой Order → пустой ID.
func (e *Encounter) Current() store.EntityID {
	if len(e.Order) == 0 {
		return ""
	}
	return e.Order[e.Turn]
}

// Advance — конец такта: следующая сущность в порядке, при wrap — новый раунд.
// Флаги действий сбрасываются автоматически: новый такт — новые ресурсы.
func (e *Encounter) Advance() {
	if len(e.Order) == 0 {
		return
	}
	e.Turn = (e.Turn + 1) % len(e.Order)
	if e.Turn == 0 {
		e.Round++
	}
	e.Reset()
}

// Reset — обнуляет ActionState. Нужен и как явный шаг сценария (например,
// при появлении haste-эффектов позже), поэтому не приватный.
func (e *Encounter) Reset() {
	e.Actions = ActionState{}
}
