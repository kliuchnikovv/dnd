package store

import (
	"encoding/json"
	"testing"
)

// cmd — заготовка команды журнала. Seq не заполняется намеренно: его назначает
// стор, иначе порядок ходов станет договорённостью вызывающей стороны.
func cmd(session SessionID, key string) CommandLogEntry {
	return CommandLogEntry{
		SessionID:      session,
		SnapshotID:     "harbour@deadbeef",
		CoreVersion:    "core-1",
		Intent:         json.RawMessage(`{"verb":"examine"}`),
		DiceCtx:        DiceCtx{Seed: 3, Turn: 1},
		IdempotencyKey: key,
	}
}

// Свежая база журнала не содержит: он заводится записью хода, а не запуском.
func TestJournalTablesStartEmpty(t *testing.T) {
	db := NewDB()
	if n := len(db.CommandLog); n != 0 {
		t.Errorf("command_log не пуст: %d", n)
	}
	if n := len(db.Audit); n != 0 {
		t.Errorf("audit не пуст: %d", n)
	}
}

// Порядковый номер назначает стор и делает это монотонно: реплей воспроизводит
// сессию по порядку интентов, и дыра в нумерации ломает именно его.
func TestAppendCommandAssignsSeqInOrder(t *testing.T) {
	db := NewDB()
	for i, key := range []string{"k1", "k2", "k3"} {
		got, appended := db.AppendCommand(cmd("s1", key))
		if !appended {
			t.Fatalf("%s: запись не добавлена", key)
		}
		if want := i + 1; got.Seq != want {
			t.Errorf("%s: seq = %d, ожидался %d", key, got.Seq, want)
		}
	}
}

// Чтение идёт по Seq, а не по порядку появления в таблице: append-only слайс —
// деталь хранения, порядок реплея задаёт номер.
func TestCommandsAreReadBackInSeqOrder(t *testing.T) {
	db := NewDB()
	db.AppendCommand(cmd("s1", "k1"))
	db.AppendCommand(cmd("s2", "other"))
	db.AppendCommand(cmd("s1", "k2"))

	got := db.Commands("s1")
	if len(got) != 2 {
		t.Fatalf("команд сессии: %d, ожидалось 2", len(got))
	}
	if got[0].Seq != 1 || got[1].Seq != 2 {
		t.Errorf("порядок seq: %d, %d", got[0].Seq, got[1].Seq)
	}
	if got[0].IdempotencyKey != "k1" || got[1].IdempotencyKey != "k2" {
		t.Errorf("перепутаны записи: %s, %s", got[0].IdempotencyKey, got[1].IdempotencyKey)
	}
}

// Нумерация локальна для сессии: чужая сессия не сдвигает порядок этой,
// иначе многопартийность позже перепишет прошлые реплеи.
func TestJournalIsPerSession(t *testing.T) {
	db := NewDB()
	db.AppendCommand(cmd("s1", "k1"))
	db.AppendCommand(cmd("s1", "k2"))

	second, _ := db.AppendCommand(cmd("s2", "k3"))
	if second.Seq != 1 {
		t.Errorf("seq второй сессии = %d, ожидался 1", second.Seq)
	}
	if n := len(db.Commands("s2")); n != 1 {
		t.Errorf("в s2 протекли чужие команды: %d", n)
	}
	if n := len(db.PendingTail("s2")); n != 1 {
		t.Errorf("в хвост s2 протекли чужие команды: %d", n)
	}
}

// Ключ идемпотентности: реплей хвоста не применяет ход дважды. Повтор не
// добавляет строку и возвращает уже записанную.
func TestAppendCommandIsIdempotentOnKey(t *testing.T) {
	db := NewDB()
	first, _ := db.AppendCommand(cmd("s1", "k1"))

	again, appended := db.AppendCommand(cmd("s1", "k1"))
	if appended {
		t.Error("повтор с тем же ключом добавил вторую строку")
	}
	if again.Seq != first.Seq {
		t.Errorf("повтор получил другой seq: %d вместо %d", again.Seq, first.Seq)
	}
	if n := len(db.CommandLog); n != 1 {
		t.Errorf("строк в журнале: %d, ожидалась 1", n)
	}
}

// Запись до обработки: команда появляется в журнале как pending, чем бы её ни
// пометила вызывающая сторона. applied ставит только MarkApplied — после Apply.
func TestAppendCommandForcesPendingStatus(t *testing.T) {
	db := NewDB()
	e := cmd("s1", "k1")
	e.Status = CommandApplied

	got, _ := db.AppendCommand(e)
	if got.Status != CommandPending {
		t.Errorf("статус при записи = %q, ожидался %q", got.Status, CommandPending)
	}
	if db.CommandLog[0].Status != CommandPending {
		t.Errorf("в таблице лежит %q", db.CommandLog[0].Status)
	}
}

func TestMarkAppliedFlipsStatus(t *testing.T) {
	db := NewDB()
	e, _ := db.AppendCommand(cmd("s1", "k1"))

	if err := db.MarkApplied("s1", e.Seq); err != nil {
		t.Fatalf("MarkApplied: %v", err)
	}
	if got := db.Commands("s1")[0].Status; got != CommandApplied {
		t.Errorf("статус = %q, ожидался %q", got, CommandApplied)
	}
}

// Неизвестная команда — явная ошибка, а не молчаливый дрейф: иначе журнал
// разойдётся с состоянием, и расхождение обнаружится только на реплее.
func TestMarkAppliedOnUnknownSeqFails(t *testing.T) {
	db := NewDB()
	db.AppendCommand(cmd("s1", "k1"))

	if err := db.MarkApplied("s1", 42); err == nil {
		t.Error("неизвестный seq прошёл без ошибки")
	}
	if err := db.MarkApplied("s2", 1); err == nil {
		t.Error("чужая сессия прошла без ошибки")
	}
}

// Повторная отметка — no-op: реплей хвоста pending обязан быть идемпотентным
// и не имеет права падать на уже применённой команде.
func TestMarkAppliedTwiceIsNoop(t *testing.T) {
	db := NewDB()
	e, _ := db.AppendCommand(cmd("s1", "k1"))

	if err := db.MarkApplied("s1", e.Seq); err != nil {
		t.Fatalf("первая отметка: %v", err)
	}
	if err := db.MarkApplied("s1", e.Seq); err != nil {
		t.Errorf("повторная отметка вернула ошибку: %v", err)
	}
	if n := len(db.CommandLog); n != 1 {
		t.Errorf("строк в журнале: %d, ожидалась 1", n)
	}
}

// Хвост pending — то, что нужно доиграть после падения между «записал» и
// «применил». Применённое из хвоста уходит.
func TestPendingTailReturnsUnfinishedInOrder(t *testing.T) {
	db := NewDB()
	db.AppendCommand(cmd("s1", "k1"))
	db.AppendCommand(cmd("s1", "k2"))
	db.AppendCommand(cmd("s1", "k3"))

	if err := db.MarkApplied("s1", 1); err != nil {
		t.Fatal(err)
	}
	tail := db.PendingTail("s1")
	if len(tail) != 2 {
		t.Fatalf("хвост: %d записей, ожидалось 2", len(tail))
	}
	if tail[0].Seq != 2 || tail[1].Seq != 3 {
		t.Errorf("порядок хвоста: %d, %d", tail[0].Seq, tail[1].Seq)
	}

	if err := db.MarkApplied("s1", 2); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkApplied("s1", 3); err != nil {
		t.Fatal(err)
	}
	if n := len(db.PendingTail("s1")); n != 0 {
		t.Errorf("после применения всего хвост непуст: %d", n)
	}
}

// Аудит-поток ссылается на команду по seq: он не источник правды реплея, но
// обязан отвечать на вопрос «что предложили против того, что применили».
func TestAppendAuditReferencesCommandBySeq(t *testing.T) {
	db := NewDB()
	e, _ := db.AppendCommand(cmd("s1", "k1"))

	db.AppendAudit(AuditEntry{
		SessionID:   "s1",
		Seq:         e.Seq,
		RawInput:    "осмотреть тело",
		LLMRole:     "intent_parser",
		LLMProposal: `{"verb":"examine","target":"e_body"}`,
		CoreVerdict: "partial",
	})
	db.AppendAudit(AuditEntry{SessionID: "s2", Seq: 1, RawInput: "чужое"})

	got := db.AuditFor("s1", e.Seq)
	if len(got) != 1 {
		t.Fatalf("записей аудита: %d, ожидалась 1", len(got))
	}
	if got[0].RawInput != "осмотреть тело" {
		t.Errorf("сырой ввод потерян: %q", got[0].RawInput)
	}
	if got[0].LLMProposal == "" {
		t.Error("предложение модели не записано — инъекцию будет нечем разбирать")
	}
}

// Имена колонок — контракт переезда в Postgres, а не деталь реализации.
func TestJournalJSONTags(t *testing.T) {
	b, err := json.Marshal(CommandLogEntry{
		SessionID:      "s1",
		Seq:            1,
		SnapshotID:     "harbour@deadbeef",
		CoreVersion:    "core-1",
		Intent:         json.RawMessage(`{"verb":"examine"}`),
		DiceCtx:        DiceCtx{Seed: 3, Turn: 1},
		IdempotencyKey: "k1",
		Status:         CommandPending,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"session_id":"s1","seq":1,"snapshot_id":"harbour@deadbeef",` +
		`"core_version":"core-1","intent":{"verb":"examine"},` +
		`"dice_ctx":{"seed":3,"turn":1},"idempotency_key":"k1","status":"pending"}`
	if got := string(b); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	b, err = json.Marshal(AuditEntry{
		SessionID:   "s1",
		Seq:         1,
		RawInput:    "осмотреть тело",
		LLMRole:     "intent_parser",
		LLMProposal: "{}",
		CoreVerdict: "partial",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want = `{"session_id":"s1","seq":1,"raw_input":"осмотреть тело",` +
		`"llm_role":"intent_parser","llm_proposal":"{}","core_verdict":"partial"}`
	if got := string(b); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
