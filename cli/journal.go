package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/llm"
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
	// w — файл журнала, если сессию пишут на диск. Строка уходит туда сразу:
	// журнал, читаемый только после закрытия файла, не спасает от падения
	// посреди хода.
	w io.Writer
	// werr — первая поломка записи. Липкая: журнал с дырой воспроизводит не ту
	// сессию, которую записывали.
	werr error
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
	e, appended := j.db.AppendCommand(store.CommandLogEntry{
		SessionID:      j.session,
		SnapshotID:     j.snapshot,
		CoreVersion:    core.Version,
		Intent:         payload,
		DiceCtx:        dice,
		IdempotencyKey: idempotencyKey(j.session, dice, payload),
	})
	if appended {
		j.write(journalRecord{Kind: recordCommand, Command: &e})
	}
	return e, j.werr
}

// commit отмечает команду применённой. Отказ мира тоже применён: запись до
// обработки не умеет знать заранее, что ядро откажет, а оставить такую команду
// в хвосте pending значит переиграть её на следующем старте.
func (j *Journal) commit(e store.CommandLogEntry) error {
	if j == nil || e.Seq == 0 {
		return nil
	}
	if err := j.db.MarkApplied(e.SessionID, e.Seq); err != nil {
		return err
	}
	j.write(journalRecord{Kind: recordApplied, Seq: e.Seq})
	return j.werr
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

// llmProposal — то, что модель предложила по недоверенному вводу. Лежит в
// аудите как JSON, а не как текст: роль говорит, кто предложил, а форма —
// разбор это был, реплика или вопрос игроку.
type llmProposal struct {
	Intent  *core.Intent `json:"intent,omitempty"`
	Reply   string       `json:"reply,omitempty"`
	Clarify string       `json:"clarify,omitempty"`
	Line    string       `json:"line,omitempty"`
	// Probe — свободная проба: ввод, который словарь не выразил и который
	// приземлился в фикшен. Своё поле, а не Clarify: спор о пробе идёт о том,
	// что модель поняла из фразы, а не о том, что игра спросила у игрока.
	Probe string `json:"probe,omitempty"`
	// Mutation — предложенное изменение мира: не команда игрока, а расширение
	// канона. Своё поле, потому что разбирают их по-разному: у команды спорят
	// о разборе фразы, у мутации — о том, что модель решила про мир.
	Mutation *proposedMutation `json:"mutation,omitempty"`
}

// proposedMutation — предложение мутации в форме аудита. Копия трёх полей
// core.Mutation, а не она сама: в колонке разбора должно лежать то, о чём
// спорят, а не структура ядра целиком с числовыми полями, которых у канона
// нет.
type proposedMutation struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Text   string `json:"text,omitempty"`
}

func (p llmProposal) encode() string {
	b, err := json.Marshal(p)
	if err != nil {
		// Аудит не отказывает: потерять след недоверенного ввода хуже, чем
		// записать его криво.
		return err.Error()
	}
	return string(b)
}

// audit пишет строку аудит-потока. Ссылка на команду — seq; нулевой seq
// означает, что команды не было: ход через модель прошёл, а до ядра не дошёл.
func (j *Journal) audit(e store.AuditEntry) {
	if j == nil {
		return
	}
	e.SessionID = j.session
	j.db.AppendAudit(e)
	j.write(journalRecord{Kind: recordAudit, Audit: &e})
}

// AuditMutation пишет в аудит вердикт ядра по предложенной мутации.
//
// Это и есть та точка, где «предложили» расходится с «применили» (ADR-0002):
// граница мутаций возвращает applied либо refusal, и без этой строки отказ
// исчезал бы бесследно — реплика персонажа в аудите есть, а отвергнутая
// деталь мира не оставляла следа нигде.
//
// Причина отказа лежит в той же колонке, что и класс исхода, с префиксом
// refused: вердикт без причины не годится для разбора, а второй колонки под
// него в схеме нет.
func (s *Session) AuditMutation(role llm.Role, m core.Mutation, app core.Applied, ref core.Refusal) {
	verdict := "applied"
	switch {
	case ref.Refused():
		verdict = "refused: " + ref.Reason
	case app.Text != "" && app.Text != strings.TrimSpace(m.Text):
		// Предложение принято, но мир от него не изменился: на занятой теме
		// действует решённое раньше. Для разбора это отдельный случай — модель
		// решила про мир иначе, чем мир уже решил.
		verdict = "applied: действующее значение сильнее предложенного"
	}
	proposal := llmProposal{Mutation: &proposedMutation{
		Kind: string(m.Kind), Target: m.Target, Text: m.Text,
	}}
	s.journal.audit(store.AuditEntry{
		Seq:         s.auditSeq,
		RawInput:    s.raw,
		LLMRole:     string(role),
		LLMProposal: proposal.encode(),
		CoreVerdict: verdict,
	})
}

// verdictOf — вердикт ядра словами данных, а не вывода. Класс исхода печатается
// игроку по-русски, но в колонке аудита должно лежать значение, по которому
// потом фильтруют инциденты, а не строка интерфейса.
func verdictOf(res core.TurnResult) string {
	switch {
	case res.Refused:
		return "refused"
	case res.Res == nil:
		// Ход без броска: ядро его применило, класса исхода у него нет.
		return "applied"
	case res.Res.Class == core.OutcomeCrit:
		return "crit"
	case res.Res.Class == core.OutcomeSuccess:
		return "success"
	case res.Res.Class == core.OutcomePartial:
		return "partial"
	default:
		return "fail"
	}
}
