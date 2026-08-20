package cli

import (
	"context"
	"fmt"
	"unicode"

	"github.com/kliuchnikovv/dnd/core"
)

// Interpreter — необязательный переводчик свободного текста в интент.
// Структурированный ввод остаётся основным: он детерминирован, и на нём
// держится воспроизводимость прогона по паре (seed, скрипт). Переводчик
// подключается только там, где структурированный парсер отбил ввод, —
// то есть договаривается на границе словаря вместо отказа.
type Interpreter interface {
	// Interpret возвращает либо интент, либо вопрос игроку. Ошибка означает
	// сбой канала, а не непонятый ввод: непонятое — это вопрос.
	Interpret(ctx context.Context, text string) (*core.Intent, string, error)
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

func (s *Session) interpret(text string, parseErr error) bool {
	if meaningless(text) {
		fmt.Fprintln(s.Out, "не понял — напиши, что ты делаешь, или скажи что-нибудь в кавычках")
		return true
	}
	if s.interp == nil {
		fmt.Fprintf(s.Out, "нельзя: %v\n", parseErr)
		return false
	}
	in, clarify, err := s.interp.Interpret(s.turnContext(), text)
	switch {
	case err != nil:
		// Сбой канала не должен выглядеть как отказ мира: игрок обязан
		// понимать, что дело в инструменте, а не в его замысле.
		fmt.Fprintf(s.Out, "переводчик недоступен: %v\nнельзя: %v\n", err, parseErr)
		return false
	case in != nil:
		s.applyIntent(*in)
		return true
	default:
		fmt.Fprintf(s.Out, "%s\n", fallbackClarify(clarify))
		return true
	}
}

func fallbackClarify(s string) string {
	if s == "" {
		return "уточни, что именно ты делаешь"
	}
	return s
}
