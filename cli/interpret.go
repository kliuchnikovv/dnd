package cli

import (
	"context"
	"strings"
	"unicode"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// Interpreter — необязательный переводчик свободного текста в интент.
// Структурированный ввод остаётся основным: он детерминирован, и на нём
// держится воспроизводимость прогона по паре (seed, скрипт). Переводчик
// подключается только там, где структурированный парсер отбил ввод, —
// то есть договаривается на границе словаря вместо отказа.
type Interpreter interface {
	// Interpret возвращает либо интент, либо вопрос игроку. Ошибка означает
	// сбой канала, а не непонятый ввод: непонятое — это вопрос.
	//
	// with — с кем игрок разговаривает, pending — вопрос, который игра задала
	// ему на прошлом ходу. Без них разбор не понимает ответа на собственный
	// вопрос: «рукой» после «чем именно?» читается как новое действие, и игра
	// спрашивает то же самое по кругу.
	Interpret(ctx context.Context, text string, with store.EntityID,
		pending string) (*core.Intent, string, error)
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
	if s.interp == nil {
		s.emit(EventRefusal, "нельзя: %v\n", parseErr)
		return false
	}
	in, clarify, err := s.interp.Interpret(s.turnContext(), text, s.spokenTo, s.pending)
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
		s.applyIntent(*in)
		return true
	default:
		question := fallbackClarify(clarify)
		// Помним, о чём спросили: следующая фраза игрока — ответ на это.
		s.pending = question
		s.emit(EventPrompt, "%s\n", question)
		return true
	}
}

func fallbackClarify(s string) string {
	if s == "" {
		return "уточни, что именно ты делаешь"
	}
	return s
}
