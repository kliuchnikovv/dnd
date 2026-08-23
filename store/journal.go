package store

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Журнал действий (ADR-0002). Два потока по назначению: журнал команд — правда
// реплея, аудит — правда разбора инцидентов. Оба append-only и в форме будущей
// схемы Postgres: ключ (session_id, seq), интент — jsonb-колонка.

type SessionID string

// CommandStatus — состояние команды. Запись до обработки означает, что строка
// появляется как pending раньше мутации ядра, а applied ставится после неё.
type CommandStatus string

const (
	CommandPending CommandStatus = "pending"
	CommandApplied CommandStatus = "applied"
)

// DiceCtx — контекст детерминизма хода: seed сессии и номер хода. Того же
// броска хватает, чтобы повторить кость: стримы именованы и детерминированы.
type DiceCtx struct {
	Seed int64 `json:"seed"`
	Turn int   `json:"turn"`
}

// CommandLogEntry — строка журнала команд.
//
// Intent лежит сырым JSON, а не структурой: store не импортирует core (стрелка
// зависимостей однонаправленная), а вторая копия формы интента внутри store
// разошлась бы с настоящей. В Postgres это jsonb-колонка.
type CommandLogEntry struct {
	SessionID      SessionID       `json:"session_id"`
	Seq            int             `json:"seq"`
	SnapshotID     string          `json:"snapshot_id"`
	CoreVersion    string          `json:"core_version"`
	Intent         json.RawMessage `json:"intent"`
	DiceCtx        DiceCtx         `json:"dice_ctx"`
	IdempotencyKey string          `json:"idempotency_key"`
	Status         CommandStatus   `json:"status"`
}

// AuditEntry — строка аудит-потока: сырой ввод игрока, предложение модели и
// вердикт ядра. Не источник правды реплея — источник правды разбора: без него
// инъекцию в --nl нечем отличить от честного хода постфактум.
type AuditEntry struct {
	SessionID   SessionID `json:"session_id"`
	Seq         int       `json:"seq"`
	RawInput    string    `json:"raw_input"`
	LLMRole     string    `json:"llm_role"`
	LLMProposal string    `json:"llm_proposal"`
	CoreVerdict string    `json:"core_verdict"`
}

// AppendCommand пишет команду как pending и возвращает записанную строку.
// Порядковый номер назначает стор: порядок реплея не может быть договорённостью
// вызывающей стороны. Второй результат — добавилась ли строка: при уже
// известном ключе идемпотентности возвращается существующая запись и false,
// чтобы реплей хвоста не применил ход дважды.
func (db *DB) AppendCommand(e CommandLogEntry) (CommandLogEntry, bool) {
	if e.IdempotencyKey != "" {
		for _, have := range db.CommandLog {
			if have.SessionID == e.SessionID && have.IdempotencyKey == e.IdempotencyKey {
				return have, false
			}
		}
	}
	e.Seq = db.nextSeq(e.SessionID)
	e.Status = CommandPending
	db.CommandLog = append(db.CommandLog, e)
	return e, true
}

// nextSeq — следующий номер внутри сессии. Нумерация локальна: чужая сессия не
// сдвигает порядок этой, иначе многопартийность перепишет прошлые реплеи.
func (db *DB) nextSeq(s SessionID) int {
	max := 0
	for _, have := range db.CommandLog {
		if have.SessionID == s && have.Seq > max {
			max = have.Seq
		}
	}
	return max + 1
}

// MarkApplied переводит команду pending → applied. Повтор — no-op: реплей
// хвоста обязан быть идемпотентным. Неизвестная команда — ошибка, а не
// молчаливый дрейф: расхождение журнала с состоянием должно быть слышно сразу.
func (db *DB) MarkApplied(s SessionID, seq int) error {
	for i := range db.CommandLog {
		if db.CommandLog[i].SessionID != s || db.CommandLog[i].Seq != seq {
			continue
		}
		db.CommandLog[i].Status = CommandApplied
		return nil
	}
	return fmt.Errorf("команда не найдена: сессия %q, seq %d", s, seq)
}

// AppendAudit пишет строку аудита. Без проверок и без отказов: аудит не источник
// правды реплея, и терять его на валидации — терять единственный след
// недоверенного ввода.
func (db *DB) AppendAudit(e AuditEntry) {
	db.Audit = append(db.Audit, e)
}

// Commands — команды сессии в порядке Seq. Append-only слайс — деталь хранения,
// порядок реплея задаёт номер.
func (db *DB) Commands(s SessionID) []CommandLogEntry {
	return db.commands(s, func(CommandLogEntry) bool { return true })
}

// PendingTail — незавершённые команды сессии: то, что нужно доиграть после
// падения между «записал» и «применил».
func (db *DB) PendingTail(s SessionID) []CommandLogEntry {
	return db.commands(s, func(e CommandLogEntry) bool { return e.Status == CommandPending })
}

func (db *DB) commands(s SessionID, keep func(CommandLogEntry) bool) []CommandLogEntry {
	var out []CommandLogEntry
	for _, e := range db.CommandLog {
		if e.SessionID == s && keep(e) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

// AuditFor — аудит, привязанный к конкретной команде.
func (db *DB) AuditFor(s SessionID, seq int) []AuditEntry {
	var out []AuditEntry
	for _, e := range db.Audit {
		if e.SessionID == s && e.Seq == seq {
			out = append(out, e)
		}
	}
	return out
}
