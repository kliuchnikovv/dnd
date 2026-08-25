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

// Своя строка обязана быть в транскрипте: игрок должен видеть, что он писал,
// и сверять это с тем, как игра его поняла. Построчный режим печатает ввод
// терминалом, полноэкранный — не печатал никак.
func TestTranscriptShowsPlayerInput(t *testing.T) {
	var tr Transcript
	tr.AppendInput("осмотреть бочки")

	out := tr.Render(60)
	if !strings.Contains(out, cli.PlayerName) {
		t.Errorf("эхо ввода не подписано игроком:\n%s", out)
	}
	if !strings.Contains(out, "осмотреть бочки") {
		t.Errorf("введённая строка не показана:\n%s", out)
	}
}

// Ход say печатает реплику игрока сам. Вместе с эхом это одна и та же фраза
// дважды подряд — и второй раз она уже ничего не сообщает.
func TestTranscriptDropsSpeechEqualToEcho(t *testing.T) {
	var tr Transcript
	tr.AppendInput("добрый день")
	tr.Append(cli.Event{Kind: cli.EventSpeech, Speaker: cli.PlayerName, Text: "— добрый день\n"})

	if got := strings.Count(tr.Render(60), "добрый день"); got != 1 {
		t.Errorf("фраза игрока показана %d раз, ждали один:\n%s", got, tr.Render(60))
	}
}

// Дедупликация — точное сравнение строк, а не догадка о смысле. Команда
// «скажи Берну "…"» и сказанное различны, и видеть надо оба: так игрок
// узнаёт, что именно из его команды дошло до персонажа.
func TestTranscriptKeepsSpeechDifferentFromEcho(t *testing.T) {
	var tr Transcript
	tr.AppendInput(`скажи Берну "добрый день"`)
	tr.Append(cli.Event{Kind: cli.EventSpeech, Speaker: cli.PlayerName, Text: "— добрый день\n"})

	out := tr.Render(60)
	if !strings.Contains(out, `скажи Берну "добрый день"`) {
		t.Errorf("команда потерялась:\n%s", out)
	}
	if !strings.Contains(out, "— добрый день") {
		t.Errorf("реплика потерялась:\n%s", out)
	}
}

// Снимается ровно ближайший дубль. Та же фраза, сказанная снова через
// несколько ходов, — новое событие разговора, а не эхо.
func TestTranscriptDropsOnlyTheAdjacentDuplicate(t *testing.T) {
	var tr Transcript
	tr.AppendInput("добрый день")
	tr.Append(cli.Event{Kind: cli.EventSpeech, Speaker: cli.PlayerName, Text: "— добрый день\n"})
	tr.Append(cli.Event{Kind: cli.EventSpeech, Speaker: "Берн", Text: "— И вам.\n"})
	tr.AppendInput("добрый день")
	tr.Append(cli.Event{Kind: cli.EventSpeech, Speaker: cli.PlayerName, Text: "— добрый день\n"})

	if got := strings.Count(tr.Render(60), "добрый день"); got != 2 {
		t.Errorf("фраза показана %d раз, ждали два:\n%s", got, tr.Render(60))
	}
}

// Отказ, произнесённый Мастером, получает метку там же, где проза: без неё в
// чате он читается как служебная строка неизвестно от кого.
func TestTranscriptLabelsRefusalByItsSpeaker(t *testing.T) {
	var tr Transcript
	tr.Append(cli.Event{Kind: cli.EventRefusal, Speaker: cli.MasterName,
		Text: "Дверь не поддаётся.\n"})
	if !strings.Contains(tr.Render(60), cli.MasterName) {
		t.Errorf("отказ не подписан автором:\n%s", tr.Render(60))
	}
}

// Служебный отказ («не понял», «переводчик недоступен») автора не получает:
// это про инструмент, а не про мир, и подписывать его Мастером значит врать
// игроку о причине.
func TestTranscriptLeavesToolRefusalUnsigned(t *testing.T) {
	var tr Transcript
	tr.Append(cli.Event{Kind: cli.EventRefusal, Text: "переводчик недоступен\n"})
	if strings.Contains(tr.Render(60), cli.MasterName) {
		t.Errorf("служебный отказ подписан Мастером:\n%s", tr.Render(60))
	}
}

// Реплика в кавычках — самый частый способ заговорить, и точное сравнение на
// ней ломалось: игрок печатает «"добрый день"», а ход say печатает «— добрый
// день». Живой прогон получил из этого дубль.
func TestTranscriptDropsSpeechEqualToQuotedEcho(t *testing.T) {
	for _, echo := range []string{
		`"добрый день"`,
		`«добрый день»`,
		`  "добрый день"  `,
	} {
		var tr Transcript
		tr.AppendInput(echo)
		tr.Append(cli.Event{Kind: cli.EventSpeech, Speaker: cli.PlayerName,
			Text: "— добрый день\n"})
		if got := strings.Count(tr.Render(60), "добрый день"); got != 1 {
			t.Errorf("эхо %q: фраза показана %d раз, ждали один:\n%s",
				echo, got, tr.Render(60))
		}
	}
}

// Команда с кавычками и сказанное всё равно различны: кавычки снимаются, но
// «скажи Берну» остаётся, и обе строки видны.
func TestTranscriptKeepsCommandWithQuotedSpeech(t *testing.T) {
	var tr Transcript
	tr.AppendInput(`скажи Берну "добрый день"`)
	tr.Append(cli.Event{Kind: cli.EventSpeech, Speaker: cli.PlayerName,
		Text: "— добрый день\n"})
	if got := strings.Count(tr.Render(60), "добрый день"); got != 2 {
		t.Errorf("строк с фразой %d, ждали две:\n%s", got, tr.Render(60))
	}
}

// Чутьё подписано своим голосом: подсказку нельзя принять ни за слова
// Мастера, ни за чью-то реплику. Раньше её произносил напарник, и она была
// речью NPC — то есть выглядела как то, за что персонаж отвечает.
func TestTranscriptLabelsHunchByItsOwnVoice(t *testing.T) {
	var tr Transcript
	tr.Append(cli.Event{Kind: cli.EventHunch, Speaker: cli.HunchName,
		Text: cli.HunchMark + "Шея. На шею так и не посмотрели.\n"})

	out := tr.Render(60)
	if !strings.Contains(out, cli.HunchName) {
		t.Errorf("подсказка не помечена чутьём:\n%s", out)
	}
	if strings.Contains(out, MasterName) {
		t.Errorf("подсказка приписана Мастеру:\n%s", out)
	}
	// Метка одна: и в строке, и блоком сверху — это то же удвоение имени,
	// каким страдала подсказка напарника.
	if got := strings.Count(out, cli.HunchName); got != 1 {
		t.Errorf("метка чутья встречается %d раз, ждали одну:\n%s", got, out)
	}
}
