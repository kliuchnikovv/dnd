package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// Journal ведёт журнал действий сессии (ADR-0002). Живёт здесь, а не в ядре:
// ядро остаётся чистым и о персистентности не знает, а запись вокруг вызова
// Apply — работа оркестратора.
//
// Нулевой указатель — рабочее состояние: сессия без журнала ничего не пишет и
// печатает то же самое. Игра без надстроек обязана работать как работала.
type Journal struct {
	db      *store.DB
	session store.SessionID
	// snapshot — из чего сессия началась: дело плюс seed. Реплей стартует
	// именно с него, поэтому идентификатор лежит в каждой строке.
	snapshot string
	seed     int64
}

func NewJournal(db *store.DB, session store.SessionID, snapshot string, seed int64) *Journal {
	return &Journal{db: db, session: session, snapshot: snapshot, seed: seed}
}

// loggedCommand — то, что лежит в jsonb-колонке команды: разобранный интент
// плюс форма обвинения, которая интентом не выражается. Обвинение — ход,
// заканчивающий дело, и без формы реплей его не повторит.
type loggedCommand struct {
	core.Intent
	Form *accusation.Form `json:"form,omitempty"`
}

// begin пишет команду до обработки: строка появляется как pending раньше, чем
// ядро тронуло состояние. Возвращённая строка нужна, чтобы отметить её
// применённой после Apply.
func (j *Journal) begin(in core.Intent, form *accusation.Form, turn int) (store.CommandLogEntry, error) {
	if j == nil {
		return store.CommandLogEntry{}, nil
	}
	payload, err := json.Marshal(loggedCommand{Intent: in, Form: form})
	if err != nil {
		// Записать нечего — и молчать об этом нельзя: ход уйдёт в ядро без
		// следа в журнале, а реплей потом не сойдётся.
		return store.CommandLogEntry{}, fmt.Errorf("интент не сериализуется: %w", err)
	}
	dice := store.DiceCtx{Seed: j.seed, Turn: turn}
	e, _ := j.db.AppendCommand(store.CommandLogEntry{
		SessionID:      j.session,
		SnapshotID:     j.snapshot,
		CoreVersion:    core.Version,
		Intent:         payload,
		DiceCtx:        dice,
		IdempotencyKey: idempotencyKey(j.session, dice, payload),
	})
	return e, nil
}

// commit отмечает команду применённой. Отказ мира тоже применён: запись до
// обработки не умеет знать заранее, что ядро откажет, а оставить такую команду
// в хвосте pending значит переиграть её на следующем старте.
func (j *Journal) commit(e store.CommandLogEntry) error {
	if j == nil || e.Seq == 0 {
		return nil
	}
	return j.db.MarkApplied(e.SessionID, e.Seq)
}

// idempotencyKey — детерминированный ключ хода: сессия, контекст кости и сам
// интент. Детерминированный, потому что реплей обязан узнать в записанной
// команде ту же самую, а не записать её второй раз.
func idempotencyKey(s store.SessionID, d store.DiceCtx, payload []byte) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%d|%d|", s, d.Seed, d.Turn)
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))[:16]
}
