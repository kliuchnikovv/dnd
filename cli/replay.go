package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// ErrNoJournal — реплей без журнала. Отказ, а не работа молча: без журнала
// нечем сверить снепшот и seed, а реплей, который не сверяет, воспроизводит
// не сессию, а совпадение.
var ErrNoJournal = errors.New("реплей без журнала: снепшот и seed сверять нечем")

// Replay прогоняет журнал команд по этой сессии: (дело + seed + журнал) →
// состояние сессии. Сырой ввод игрока для этого не нужен и не используется —
// правда реплея это структурный интент (ADR-0002).
//
// Реплей идёт ТЕМ ЖЕ путём, что живой ход, а не отдельной веткой. Так надо:
// перемещение живёт в afterAction (известный шов — ядро резолвит бросок,
// презентация меняет узел), и реплей в обход презентации оставил бы игрока
// стоять на месте всю записанную сессию.
//
// Модели во время реплея быть не должно: озвучка недетерминирована и стоит
// денег, а на состояние не влияет. Сессию под реплей собирают без них.
//
// Пока состояние нигде не снимается, восстановление после падения — это реплей
// ВСЕГО журнала, а не только хвоста pending: в памяти после падения не остаётся
// ничего, к чему хвост можно было бы дописать. Двойного применения при этом не
// бывает по построению: реплей журнал не дописывает, а прерванная команда лишь
// получает отметку «применена».
func (s *Session) Replay(entries []store.CommandLogEntry) error {
	if s.journal == nil {
		return ErrNoJournal
	}
	for _, e := range entries {
		if err := s.checkEntry(e); err != nil {
			return err
		}
		var c loggedCommand
		if err := json.Unmarshal(e.Intent, &c); err != nil {
			return fmt.Errorf("seq %d: интент не разбирается: %w", e.Seq, err)
		}
		// Номер хода восстанавливается из журнала, а не считается заново: на
		// нём стоит ключ идемпотентности, и без этого реплей записал бы те же
		// команды второй раз под другими номерами.
		s.turn = e.DiceCtx.Turn
		s.raw, s.llmRole, s.llmProposal = "", "", ""
		entry := e
		s.replaying = &entry
		err := s.replayOne(c)
		s.replaying = nil
		if err != nil {
			return fmt.Errorf("seq %d: %w", e.Seq, err)
		}
	}
	return nil
}

// checkEntry сверяет заголовок записи с тем, на чём её собираются переигрывать.
// Расхождение — явная ошибка, а не молчаливый дрейф: правка правил меняет исход
// прошлых бросков, и реплей обязан сказать это вслух, а не выдать другую
// сессию за ту же.
func (s *Session) checkEntry(e store.CommandLogEntry) error {
	if e.CoreVersion != core.Version {
		return fmt.Errorf("seq %d: журнал снят на ядре %q, сейчас %q",
			e.Seq, e.CoreVersion, core.Version)
	}
	if e.SnapshotID != s.journal.snapshot {
		return fmt.Errorf("seq %d: журнал снят со снепшота %q, сессия на %q",
			e.Seq, e.SnapshotID, s.journal.snapshot)
	}
	if e.DiceCtx.Seed != s.journal.seed {
		return fmt.Errorf("seq %d: журнал снят на seed %d, сессия на %d",
			e.Seq, e.DiceCtx.Seed, s.journal.seed)
	}
	return nil
}

// replayOne исполняет одну записанную команду. Ветки — те же тела, что у
// живого ввода: расхождение между вводом и реплеем было бы багом, который
// виден только в одном из них.
func (s *Session) replayOne(c loggedCommand) error {
	switch c.Verb {
	case "accuse":
		if c.Form == nil {
			return errors.New("обвинение без формы")
		}
		s.resolveAccusation(*c.Form)
	case "rest":
		s.applyRest(c.Args.Text)
	case "compare":
		if len(c.Args.Facts) != 2 {
			return fmt.Errorf("сопоставление не пары фактов: %v", c.Args.Facts)
		}
		s.applyCompare(c.Args.Facts)
	default:
		// Интент записан уже готовым: актёр и адресат разрешены на живом
		// ходу, и разрешать их второй раз значит дать другой ответ.
		s.execute(c.Intent)
	}
	return nil
}
