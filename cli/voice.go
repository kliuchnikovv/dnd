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

// Turn — номер текущего хода. Нужен надстройкам, которые ведут память
// разговора: транскрипт читается как разговор, а не как список.
func (s *Session) Turn() int { return s.turn }

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

// Narrator — необязательная проза Мастера. Он описывает сцену и исход вместо
// статичного авторского текста, но авторский текст остаётся рамкой: Мастер её
// оживляет, не заменяя.
//
// Механические строки — бросок, «узнали», цена — Мастеру не принадлежат: их
// печатает код, иначе проза начнёт врать о механике.
type Narrator interface {
	// kind различает описание места и исход хода. Догадываться по пустому
	// исходу нельзя: у социального хода механики нет вовсе, и «поздороваться»
	// выглядело как «игрок озирается».
	Narrate(ctx context.Context, kind ProseKind, frame string,
		scene, outcome []string) (string, error)
}

// WithNarrator включает прозу Мастера.
func (s *Session) WithNarrator(n Narrator) *Session {
	s.narrator = n
	s.r.Prose = func(kind ProseKind, frame string, scene, outcome []string) string {
		// Сбой надстройки не рушит ход: печатается авторский текст, как без
		// -nl. Проза необязательна, а ход уже сыгран.
		//
		// Но молчать о сбое нельзя. Молча откатываясь, игра выглядит рабочей
		// при выключенном Мастере: код написан, вызов не доходит до модели,
		// игрок видит бледный текст и не знает, что это поломка. Именно так
		// незароученная роль прожила целую фазу.
		out, err := n.Narrate(s.turnContext(), kind, frame, scene, outcome)
		if err != nil {
			s.noteOnce("Мастер промолчал: " + err.Error())
			return frame
		}
		if strings.TrimSpace(out) == "" {
			return frame
		}
		return out
	}
	return s
}

// noteOnce сообщает о поломке надстройки один раз на прогон. Каждая строка
// прозы жаловалась бы отдельно, а шум читается хуже тишины.
func (s *Session) noteOnce(text string) {
	if s.noted == nil {
		s.noted = map[string]bool{}
	}
	if s.noted[text] {
		return
	}
	s.noted[text] = true
	fmt.Fprintf(s.Out, "(%s)\n", text)
}
