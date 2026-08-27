package tui

import (
	"strings"

	"github.com/kliuchnikovv/dnd/cli"
)

// MasterName — подпись прозы. Значение живёт в cli рядом с подписью игрока:
// два места правды о подписи разъезжаются молча, и в раскладке это видно
// сразу.
const MasterName = cli.MasterName

// Transcript — что уже сказано, в виде, готовом к показу. Копит события, а не
// строки: подпись и разделитель зависят от автора, а не от текста.
type Transcript struct {
	events []cli.Event
	// echo — последняя строка, введённая игроком. Хранится, чтобы снять
	// ровно один дубль: ход say печатает реплику игрока сам, и без этого
	// одна и та же фраза стоит в транскрипте двумя блоками подряд.
	echo string
}

// AppendInput кладёт в транскрипт то, что игрок напечатал. Эхо живёт здесь, а
// не в cli: приёмник cli.TextSink напечатал бы его, и построчный вывод
// перестал бы быть побайтово прежним — а он спецификация. Построчному режиму
// эхо и не нужно, там ввод печатает сам терминал.
func (t *Transcript) AppendInput(line string) {
	t.echo = line
	t.events = append(t.events,
		cli.Event{Kind: cli.EventSpeech, Speaker: cli.PlayerName, Text: line + "\n"})
}

func (t *Transcript) Append(e cli.Event) {
	if t.duplicatesEcho(e) {
		// Дубль снят — и только один: следующая такая же фраза это новое
		// событие разговора, а не эхо.
		t.echo = ""
		return
	}
	t.events = append(t.events, e)
}

// duplicatesEcho — это событие пересказывает только что напечатанную строку?
// Сравнение точное, по строке, а не догадка о смысле: «скажи Берну "добрый
// день"» и сказанное «добрый день» различны, и видеть надо оба — так игрок
// узнаёт, что именно из его команды дошло до персонажа.
func (t *Transcript) duplicatesEcho(e cli.Event) bool {
	if t.echo == "" || e.Kind != cli.EventSpeech || e.Speaker != cli.PlayerName {
		return false
	}
	return bareSpeech(e.Text) == bareSpeech(t.echo)
}

// bareSpeech снимает оформление реплики: тире, которым её печатает cli.Spoken,
// и кавычки, в которые её берёт игрок. Реплика в кавычках — самый частый
// способ заговорить, и без снятия точное сравнение на ней ломалось: живой
// прогон получил «"добрый день"» и «— добрый день» двумя блоками подряд.
//
// Снимается только оформление. «скажи Берну "добрый день"» после этого всё
// равно не равно «добрый день», и обе строки останутся — так и надо: игрок
// видит, что именно из его команды дошло до персонажа.
func bareSpeech(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSpace(strings.TrimPrefix(s, "—"))
	return strings.Trim(s, `"«»„“”`)
}

// Render собирает транскрипт под ширину экрана. Метка ставится при смене
// говорящего: подпись над каждой строкой одного и того же человека — шум.
func (t *Transcript) Render(width int) string {
	var b strings.Builder
	prev := ""
	for _, e := range t.events {
		who := cli.SpeakerOf(e)
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

func separator(width int) string {
	if width < 8 {
		width = 8
	}
	return strings.Repeat("─", width-2)
}
