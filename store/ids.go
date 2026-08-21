// Package store держит строки таблиц в памяти. Структура повторяет будущую
// схему Postgres: ссылки идут по идентификаторам, указателей между сущностями
// нет, чтобы переезд в базу был механическим INSERT.
package store

type (
	FactID      string
	EntityID    string
	NodeID      string
	ClockID     string
	CharacterID string
	PropID      string
	CaseID      string

	// Token — значение слота обвинения (who / how / when / why).
	// Токен не равен факту: факт лишь открывает токен к использованию.
	Token string
)
