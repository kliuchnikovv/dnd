package cli

import (
	"fmt"
	"io"
)

// EventKind — что это за строка вывода. Без вида и автора метку говорящего
// пришлось бы восстанавливать разбором напечатанного текста, то есть парсить
// собственный вывод.
type EventKind string

const (
	EventScene   EventKind = "scene"   // описание места
	EventProse   EventKind = "prose"   // проза Мастера об исходе хода
	EventSpeech  EventKind = "speech"  // прямая речь: NPC или игрок
	EventSystem  EventKind = "system"  // справка, факты, часы, состояние
	EventRefusal EventKind = "refusal" // «нельзя: …»
	EventPrompt  EventKind = "prompt"  // игра спросила: слот обвинения, уточнение
	EventNote    EventKind = "note"    // поломка надстройки: «Мастер промолчал»
)

// Event — единица вывода. Text уже отрендерен: приёмник его не собирает и не
// разбирает, а показывает. Так построчный режим остаётся побайтово прежним.
type Event struct {
	Kind    EventKind
	Speaker string // «Берн, стражник» либо «Вы»; только у EventSpeech
	Text    string
}

// Sink — куда уходит вывод сессии. Интерфейс нужен, чтобы полноэкранный режим
// был вторым приёмником, а не вторым форматом вывода.
type Sink interface{ Emit(Event) }

// TextSink печатает событие как есть. Это сегодняшнее поведение игры целиком:
// вид и автор ему не нужны, потому что текст уже собран.
type TextSink struct{ W io.Writer }

func (t TextSink) Emit(e Event) { fmt.Fprint(t.W, e.Text) }
