package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/llm"
)

// turnContext помечает контекст номером хода. Потолок вызовов модели на ход
// считается по нему: перевод и озвучка одной строки ввода — один ход.
func (s *Session) turnContext() context.Context {
	return llm.WithTurnID(context.Background(), "turn-"+strconv.Itoa(s.turn))
}

// Voicer — необязательный голос NPC. Без него игра работает как раньше,
// авторской прозой: озвучка это надстройка, а не условие работы.
type Voicer interface {
	// Voice возвращает реплику прямой речью либо пустую строку, если
	// персонажу сейчас нечего сказать.
	Voice(ctx context.Context, in core.Intent, res core.TurnResult) (string, error)
}

// WithVoicer включает реплики NPC.
func (s *Session) WithVoicer(v Voicer) *Session {
	s.voicer = v
	return s
}

// Spoken оформляет прямую речь. Одно место на всю игру: реплика NPC и
// реплика игрока должны выглядеть одинаково.
func Spoken(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	return "— " + line
}

// speak печатает реплику, если персонаж заговорил. Сбой канала не должен
// прерывать ход: озвучка необязательна, а ход уже закоммичен.
func (s *Session) speak(in core.Intent, res core.TurnResult) {
	if s.voicer == nil || res.Refused {
		return
	}
	line, err := s.voicer.Voice(s.turnContext(), in, res)
	if err != nil {
		fmt.Fprintf(s.Out, "(персонаж промолчал: %v)\n", err)
		return
	}
	if line != "" {
		fmt.Fprintln(s.Out, Spoken(line))
	}
}
