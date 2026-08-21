package store

import "encoding/json"

// Таблицы, до которых M1a не доходит. Заводятся сейчас по одной причине:
// пустая таблица стоит ноль, а миграция потом стоит дорого. Ни одна из них не
// читается движком — они держат форму будущей схемы, чтобы переезд в Postgres
// остался механическим INSERT.

type (
	RegionID string
	EventID  string
	PartyID  string
)

// Case — строка дела. Правда и вердикт разведены намеренно: вердикт попадает
// в мир и может быть неверным, правда остаётся здесь и не покидает базу.
// Ошибочный вердикт создаёт ложное закрытие, которое другая парти
// опрокидывает, — это и есть главный хук общего мира в детективе.
type Case struct {
	ID        CaseID   `json:"id"`
	RegionID  RegionID `json:"region_id"`
	Archetype string   `json:"archetype"`
	// DeductionAxis, InfoChannel, Complexity — оси, по которым генератор M1c
	// варьирует дела.
	DeductionAxis string `json:"deduction_axis"`
	InfoChannel   string `json:"info_channel"`
	Complexity    int    `json:"complexity"`
	// Truth — правда дела. НИКОГДА не попадает ни в один промпт и ни в один
	// вывод игроку.
	Truth          json.RawMessage `json:"truth"`
	Status         string          `json:"status"`
	Verdict        json.RawMessage `json:"verdict"`
	VerdictBy      PartyID         `json:"verdict_by_party"`
	VerdictCorrect bool            `json:"verdict_correct"`
}

type Region struct {
	ID   RegionID `json:"id"`
	Name string   `json:"name"`
	Kind string   `json:"kind"`
}

// WorldEvent — то, что случилось в общем мире и видно другим парти.
type WorldEvent struct {
	ID       EventID  `json:"id"`
	RegionID RegionID `json:"region_id"`
	Kind     string   `json:"kind"`
	Payload  json.RawMessage
	At       int `json:"at"`
}

// OutboxMessage — исходящий эффект, ждущий доставки. Транзакционный outbox:
// запись о событии и его публикация не должны расходиться.
type OutboxMessage struct {
	ID        EventID `json:"id"`
	Topic     string  `json:"topic"`
	Payload   json.RawMessage
	Published bool `json:"published"`
}
