package cli

import (
	"context"
	"fmt"

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
func (s *Session) interpret(text string, parseErr error) bool {
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
		in.Actor = s.Game.Actor
		res := s.Game.Apply(*in)
		fmt.Fprint(s.Out, s.r.Turn(s.Game, res))
		s.afterAction(*in, res)
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
