package tui

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cli"
)

// Метка ставится при СМЕНЕ говорящего: подпись над каждой строкой подряд
// идущих реплик одного человека — шум, из которого не видно границ реплики.
func TestTranscriptLabelsOnSpeakerChange(t *testing.T) {
	var tr Transcript
	tr.Append(cli.Event{Kind: cli.EventSpeech, Speaker: "Берн", Text: "— Первое.\n"})
	tr.Append(cli.Event{Kind: cli.EventSpeech, Speaker: "Берн", Text: "— Второе.\n"})
	tr.Append(cli.Event{Kind: cli.EventSpeech, Speaker: cli.PlayerName, Text: "— Третье.\n"})

	out := tr.Render(60)
	if got := strings.Count(out, "Берн"); got != 1 {
		t.Errorf("метка «Берн» встречается %d раз, ждали одну", got)
	}
	if !strings.Contains(out, cli.PlayerName) {
		t.Errorf("смена говорящего не отмечена:\n%s", out)
	}
}

// Проза Мастера подписана Мастером: игрок должен видеть, что это не персонаж.
func TestTranscriptLabelsMaster(t *testing.T) {
	var tr Transcript
	tr.Append(cli.Event{Kind: cli.EventProse, Text: "Дождь бьёт по доскам.\n"})
	if !strings.Contains(tr.Render(60), "Мастер") {
		t.Errorf("проза не подписана Мастером:\n%s", tr.Render(60))
	}
}

// Справка и служебные строки подписи не получают: подписывать «Мастером»
// список команд — враньё об авторстве.
func TestTranscriptLeavesSystemUnsigned(t *testing.T) {
	var tr Transcript
	tr.Append(cli.Event{Kind: cli.EventSystem, Text: "команды: look, facts\n"})
	if strings.Contains(tr.Render(60), "Мастер") {
		t.Errorf("служебный вывод подписан автором:\n%s", tr.Render(60))
	}
}
