package cli

import (
	"context"
	"fmt"

	"github.com/kliuchnikovv/dnd/core"
)

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

// speak печатает реплику, если персонаж заговорил. Сбой канала не должен
// прерывать ход: озвучка необязательна, а ход уже закоммичен.
func (s *Session) speak(in core.Intent, res core.TurnResult) {
	if s.voicer == nil || res.Refused {
		return
	}
	line, err := s.voicer.Voice(context.Background(), in, res)
	if err != nil {
		fmt.Fprintf(s.Out, "(персонаж промолчал: %v)\n", err)
		return
	}
	if line != "" {
		fmt.Fprintln(s.Out, line)
	}
}
