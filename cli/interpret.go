package cli

import (
	"context"
	"strings"
	"unicode"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/store"
)

// Interpreter — необязательный переводчик свободного текста в интент.
// Структурированный ввод остаётся основным: он детерминирован, и на нём
// держится воспроизводимость прогона по паре (seed, скрипт). Переводчик
// подключается только там, где структурированный парсер отбил ввод, —
// то есть договаривается на границе словаря вместо отказа.
type Interpreter interface {
	// Interpret возвращает интент, свободную пробу либо вопрос игроку. Ошибка
	// означает сбой канала, а не непонятый ввод: непонятое — это вопрос, а
	// невыразимое словарём — проба (ADR-0003, T1). Исхода-тупика нет.
	//
	// with — с кем игрок разговаривает, pending — вопрос, который игра задала
	// ему на прошлом ходу. Без них разбор не понимает ответа на собственный
	// вопрос: «рукой» после «чем именно?» читается как новое действие, и игра
	// спрашивает то же самое по кругу.
	Interpret(ctx context.Context, text string, with store.EntityID,
		pending string) (in *core.Intent, probe core.Probe, clarify string, err error)
}

// ChatInterpreter — переводчик чат-режима: тем же вызовом, которым разобрал
// фразу, он отвечает игроку репликой Мастера. Реплика безоценочная — об исходе
// она не знает, потому что бросок ещё не сделан, и решает его ядро.
//
// Метод возвращает пять значений, а не структуру, ровно затем, зачем
// Interpreter возвращает четыре: чтобы intent реализовал интерфейс структурно,
// не импортируя презентацию. Структура жила бы в intent — и тогда cli пришлось
// бы его импортировать, развернув направление слоёв. Порядок слоёв важнее
// краткости подписи, и это заявленная цена, а не недосмотр.
type ChatInterpreter interface {
	// InterpretChat возвращает интент, безоценочную реплику Мастера, свободную
	// пробу и вопрос игроку. Ошибка означает сбой канала, а не непонятый ввод.
	InterpretChat(ctx context.Context, text string, with store.EntityID,
		pending string) (in *core.Intent, reply string, probe core.Probe, clarify string, err error)
}

// WithChat включает чат-режим. Он идёт ВМЕСТО перевода свободного текста, а не
// вместе с ним: два переводчика на один ввод означали бы два разных разбора
// одной фразы.
func (s *Session) WithChat(c ChatInterpreter) *Session {
	s.chat = c
	return s
}

// WithInterpreter включает перевод свободного текста.
func (s *Session) WithInterpreter(i Interpreter) *Session {
	s.interp = i
	return s
}

// interpret обрабатывает ввод, который не разобрал структурированный парсер.
// Возвращает true, если ввод удалось во что-то превратить.
// meaningless — ввод, на который не стоит тратить вызов модели: знаки
// пунктуации, одна буква, пустота. Модель на такое отвечает «игрок не ввёл
// действие», и это знание не стоит своей цены.
func meaningless(text string) bool {
	letters := 0
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			letters++
		}
	}
	return letters < 2
}

// inventoryQuestions — как игрок спрашивает о том, что несёт. Список узкий
// сознательно: «достаю из кармана» это действие, а не вопрос о карманах, и
// широкий словарь отвечал бы на него списком вместо дела.
//
// Спрос об инвентаре перехватывается до переводчика: ответ у игры уже есть, и
// платить за него вызовом модели незачем. Игрок при этом не обязан знать, что
// одно спрашивается фразой, а другое — командой.
var inventoryQuestions = []string{
	"инвентар", "что у меня", "что при себе", "что несу", "что я несу",
	"мои вещи", "в карманах", "в сумке",
}

func asksAboutInventory(text string) bool {
	lower := strings.ToLower(text)
	for _, q := range inventoryQuestions {
		if strings.Contains(lower, q) {
			return true
		}
	}
	return false
}

func (s *Session) interpret(text string, parseErr error) bool {
	if asksAboutInventory(text) {
		s.emitText(EventSystem, s.r.Items(s.Game))
		return true
	}
	if meaningless(text) {
		s.emit(EventRefusal, "не понял — напиши, что ты делаешь, или скажи что-нибудь в кавычках\n")
		return true
	}
	if s.chat != nil {
		return s.interpretChat(text, parseErr)
	}
	if s.interp == nil {
		s.emit(EventRefusal, "нельзя: %v\n", parseErr)
		return false
	}
	in, probe, clarify, err := s.interp.Interpret(s.turnContext(), text, s.spokenTo, s.pending)
	// Вопрос задан один раз: ответ на него уже пришёл, и тащить его дальше
	// значит навязывать модели старый контекст.
	s.pending = ""
	switch {
	case err != nil:
		// Сбой канала не должен выглядеть как отказ мира: игрок обязан
		// понимать, что дело в инструменте, а не в его замысле.
		s.emit(EventRefusal, "переводчик недоступен: %v\nнельзя: %v\n", err, parseErr)
		return false
	case in != nil:
		s.noteProposal(llm.RoleIntentParser, llmProposal{Intent: in})
		s.applyIntent(*in)
		return true
	case probe.Text != "":
		s.resolveProbe(probe)
		return true
	default:
		question := fallbackClarify(clarify)
		// Помним, о чём спросили: следующая фраза игрока — ответ на это.
		s.pending = question
		s.emit(EventPrompt, "%s\n", question)
		// Ход не состоялся, но модель по недоверенному вводу уже
		// высказалась — и инъекция живёт ровно здесь. Строка аудита пишется
		// без команды и без вердикта: ядру этот ввод не дошёл.
		s.noteProposal(llm.RoleIntentParser, llmProposal{Clarify: question})
		s.journalAudit("")
		return true
	}
}

// interpretChat — чат-режим: один вызов даёт разбор и реплику, ядро решает,
// состоится ли ход. Порядок именно такой: реплика показывается ПОСЛЕ того, как
// ядро разрешило. Показать подводку к действию, которого не будет, значит
// соврать игроку — а ядро над Мастером, а не наоборот.
func (s *Session) interpretChat(text string, parseErr error) bool {
	in, reply, probe, clarify, err := s.chat.InterpretChat(s.turnContext(), text, s.spokenTo, s.pending)
	// Вопрос задан один раз: ответ на него уже пришёл.
	s.pending = ""
	switch {
	case err != nil:
		// Сбой канала не должен выглядеть как отказ мира: игрок обязан
		// понимать, что дело в инструменте, а не в его замысле.
		s.emit(EventRefusal, "переводчик недоступен: %v\nнельзя: %v\n", err, parseErr)
		return false
	case probe.Text != "":
		// Реплика чат-режима подводкой к пробе быть не может: подводка
		// предваряет действие, а у пробы отклик и есть весь её текст. Показать
		// оба значило бы описать одно событие дважды.
		s.noteProposal(llm.RoleChatMaster, llmProposal{Probe: probe.Text})
		s.resolveProbe(probe)
		return true
	case in == nil:
		question := fallbackClarify(clarify)
		s.pending = question
		s.emit(EventPrompt, "%s\n", question)
		s.noteProposal(llm.RoleChatMaster, llmProposal{Reply: reply, Clarify: question})
		s.journalAudit("")
		return true
	}

	s.noteProposal(llm.RoleChatMaster, llmProposal{Intent: in, Reply: reply})
	ready, ok := s.prepare(*in, "")
	if !ok {
		// Игра спросила, к кому обращён ход: он не состоится, и реплике
		// предварять нечего.
		return true
	}
	// Ядро — единственная власть над «можно», и спрашивается оно ДО показа.
	// Отказ реплику съедает; сам отказ печатает execute, он же считает
	// холостой ход, без которого чутьё молчит именно тогда, когда нужно.
	if reply != "" {
		if s.Game.Check(ready).Refused {
			s.chatEaten++
		} else {
			s.chatShown++
			s.emitSpeech(MasterName, "%s\n", reply)
		}
	}
	s.execute(ready)
	return true
}

func fallbackClarify(s string) string {
	if s == "" {
		return "уточни, что именно ты делаешь"
	}
	return s
}
