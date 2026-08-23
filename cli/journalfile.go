package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// Файл журнала — построчный JSON, а не один документ. Так надо: журнал пишется
// ДО обработки хода, и запись, которая станет читаемой только после закрытия
// файла, не спасает от падения посреди хода — то есть не спасает ни от чего.
//
// Три вида строк отражают три события: заголовок сессии, записанная команда и
// отметка «применена». Отметка отдельной строкой, потому что статус меняется
// после мутации ядра: слить её с командой значило бы писать команду после
// обработки, а это ровно то, чего WAL не делает.
const (
	recordHeader  = "header"
	recordCommand = "command"
	recordApplied = "applied"
	recordAudit   = "audit"
)

type journalRecord struct {
	Kind        string                 `json:"kind"`
	Session     store.SessionID        `json:"session,omitempty"`
	Snapshot    string                 `json:"snapshot,omitempty"`
	Seed        int64                  `json:"seed,omitempty"`
	CoreVersion string                 `json:"core_version,omitempty"`
	Seq         int                    `json:"seq,omitempty"`
	Command     *store.CommandLogEntry `json:"command,omitempty"`
	Audit       *store.AuditEntry      `json:"audit,omitempty"`
}

// JournalDump — журнал сессии, прочитанный с диска.
type JournalDump struct {
	Session     store.SessionID
	Snapshot    string
	Seed        int64
	CoreVersion string
	Commands    []store.CommandLogEntry
	Audit       []store.AuditEntry
}

// WithWriter отправляет журнал на диск построчно. Заголовок пишется сразу:
// сессия, из которой не успело выйти ни одного хода, всё равно должна быть
// опознаваема.
func (j *Journal) WithWriter(w io.Writer) *Journal {
	j.w = w
	j.write(journalRecord{
		Kind: recordHeader, Session: j.session, Snapshot: j.snapshot,
		Seed: j.seed, CoreVersion: core.Version,
	})
	return j
}

// Err — первая поломка записи на диск. Липкая: журнал, который потерял одну
// строку, дальше не журнал, и молчать об этом нельзя — на нём стоит
// воспроизводимость.
func (j *Journal) Err() error {
	if j == nil {
		return nil
	}
	return j.werr
}

func (j *Journal) write(rec journalRecord) {
	if j.w == nil || j.werr != nil {
		return
	}
	line, err := json.Marshal(rec)
	if err != nil {
		j.werr = fmt.Errorf("журнал не сериализуется: %w", err)
		return
	}
	if _, err := fmt.Fprintf(j.w, "%s\n", line); err != nil {
		j.werr = fmt.Errorf("журнал не пишется: %w", err)
	}
}

// ReadJournal читает файл журнала. Битая строка — ошибка с номером строки, а
// не пропуск: журнал с дырой воспроизводит не ту сессию, которую записывали.
func ReadJournal(r io.Reader) (JournalDump, error) {
	var dump JournalDump
	// Статусы приезжают отдельными строками, поэтому команды собираются по
	// seq, а не просто накапливаются.
	applied := map[int]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for line := 1; sc.Scan(); line++ {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var rec journalRecord
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			return JournalDump{}, fmt.Errorf("строка %d: %w", line, err)
		}
		switch rec.Kind {
		case recordHeader:
			dump.Session, dump.Snapshot = rec.Session, rec.Snapshot
			dump.Seed, dump.CoreVersion = rec.Seed, rec.CoreVersion
		case recordCommand:
			if rec.Command == nil {
				return JournalDump{}, fmt.Errorf("строка %d: команда без содержимого", line)
			}
			dump.Commands = append(dump.Commands, *rec.Command)
		case recordApplied:
			applied[rec.Seq] = true
		case recordAudit:
			if rec.Audit == nil {
				return JournalDump{}, fmt.Errorf("строка %d: аудит без содержимого", line)
			}
			dump.Audit = append(dump.Audit, *rec.Audit)
		default:
			return JournalDump{}, fmt.Errorf("строка %d: неизвестный вид записи %q", line, rec.Kind)
		}
	}
	if err := sc.Err(); err != nil {
		return JournalDump{}, err
	}
	if dump.Session == "" {
		return JournalDump{}, fmt.Errorf("журнал без заголовка: сверять снепшот и seed нечем")
	}
	for i := range dump.Commands {
		if applied[dump.Commands[i].Seq] {
			dump.Commands[i].Status = store.CommandApplied
		}
	}
	return dump, nil
}
