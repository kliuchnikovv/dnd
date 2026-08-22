package tui

import (
	"strings"

	"github.com/kliuchnikovv/dnd/cli"
)

// MasterName — подпись прозы. Мастер такой же голос, как персонаж, и игрок
// обязан видеть, кто именно с ним говорит.
const MasterName = "Мастер"

// Transcript — что уже сказано, в виде, готовом к показу. Копит события, а не
// строки: подпись и разделитель зависят от автора, а не от текста.
type Transcript struct {
	events []cli.Event
}

func (t *Transcript) Append(e cli.Event) { t.events = append(t.events, e) }

// Render собирает транскрипт под ширину экрана. Метка ставится при смене
// говорящего: подпись над каждой строкой одного и того же человека — шум.
func (t *Transcript) Render(width int) string {
	var b strings.Builder
	prev := ""
	for _, e := range t.events {
		who := speakerOf(e)
		if who != "" && who != prev {
			if b.Len() > 0 {
				b.WriteString(separator(width) + "\n")
			}
			b.WriteString(who + "\n")
		}
		// Отступ блока съедает два знака ширины, поэтому перенос считается по
		// остатку: иначе строка ровно по ширине окна уедет на край и обрежется.
		for _, line := range wrap(strings.TrimRight(e.Text, "\n"), width-2) {
			b.WriteString("  " + line + "\n")
		}
		prev = who
	}
	return b.String()
}

// speakerOf — кто автор события. Пусто у служебного вывода: подписывать
// список команд чьим-то именем значит врать об авторстве.
func speakerOf(e cli.Event) string {
	switch e.Kind {
	case cli.EventSpeech:
		return e.Speaker
	case cli.EventProse, cli.EventScene:
		return MasterName
	default:
		return ""
	}
}

func separator(width int) string {
	if width < 8 {
		width = 8
	}
	return strings.Repeat("─", width-2)
}
