package cli

import (
	"strings"
	"testing"
)

func TestParseStructuredAction(t *testing.T) {
	cmd, err := Parse("question ivar ledger")
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if cmd.Kind != CmdAction {
		t.Fatalf("вид команды %v", cmd.Kind)
	}
	if cmd.Intent.Verb != "question" {
		t.Errorf("глагол %q", cmd.Intent.Verb)
	}
	if cmd.Intent.Args.Target != "e_ivar" {
		t.Errorf("цель %q, ожидалось e_ivar", cmd.Intent.Args.Target)
	}
	if cmd.Intent.Args.Topic != "f_ledger" {
		t.Errorf("тема %q, ожидалось f_ledger", cmd.Intent.Args.Topic)
	}
}

func TestParseAcceptsFullIdentifiers(t *testing.T) {
	// Игрок может ввести и короткое имя, и полный id из вывода facts.
	cmd, _ := Parse("question e_ivar f_ledger")
	if cmd.Intent.Args.Target != "e_ivar" || cmd.Intent.Args.Topic != "f_ledger" {
		t.Errorf("полные идентификаторы искажены: %+v", cmd.Intent.Args)
	}
}

func TestParseSingleArgVerbs(t *testing.T) {
	cases := map[string]struct{ verb, target string }{
		"examine body":     {"examine", "e_body"},
		"search warehouse": {"search", "e_warehouse"},
		"stake_out quay":   {"stake_out", "e_quay"},
	}
	for line, want := range cases {
		cmd, err := Parse(line)
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		if string(cmd.Intent.Verb) != want.verb || string(cmd.Intent.Args.Target) != want.target {
			t.Errorf("%q -> %q %q", line, cmd.Intent.Verb, cmd.Intent.Args.Target)
		}
	}
}

func TestParseCompareTakesTwoFacts(t *testing.T) {
	cmd, err := Parse("compare alibi seen_at_quay")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Kind != CmdCompare {
		t.Fatalf("вид команды %v", cmd.Kind)
	}
	if len(cmd.Facts) != 2 || cmd.Facts[0] != "f_alibi" || cmd.Facts[1] != "f_seen_at_quay" {
		t.Errorf("факты = %v", cmd.Facts)
	}
}

func TestParseBareCommands(t *testing.T) {
	cases := map[string]CommandKind{
		"facts": CmdFacts, "state": CmdState, "clocks": CmdClocks,
		"help": CmdHelp, "accuse": CmdAccuse, "quit": CmdQuit,
	}
	for line, want := range cases {
		cmd, err := Parse(line)
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		if cmd.Kind != want {
			t.Errorf("%q -> %v, ожидалось %v", line, cmd.Kind, want)
		}
	}
}

func TestParseTheorizeKeepsFreeText(t *testing.T) {
	cmd, err := Parse("theorize токе подменил запись в гроссбухе")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Intent.Args.Text != "токе подменил запись в гроссбухе" {
		t.Errorf("текст гипотезы искажён: %q", cmd.Intent.Args.Text)
	}
}

func TestParseRejectsUnknownVerb(t *testing.T) {
	if _, err := Parse("interrogate ivar"); err == nil {
		t.Error("неизвестный глагол принят")
	}
}

func TestParseIgnoresBlankAndComments(t *testing.T) {
	// Скриптовый режим читает файлы с комментариями — они не должны быть ходами.
	for _, line := range []string{"", "   ", "# это комментарий"} {
		cmd, err := Parse(line)
		if err != nil {
			t.Fatalf("%q дал ошибку %v", line, err)
		}
		if cmd.Kind != CmdNone {
			t.Errorf("%q разобрано как команда %v", line, cmd.Kind)
		}
	}
}

func TestParseRejectsWrongArity(t *testing.T) {
	if _, err := Parse("question ivar"); err == nil {
		t.Error("question без темы принят")
	}
	if _, err := Parse("compare alibi"); err == nil {
		t.Error("compare с одним фактом принят")
	}
}

// --- прямая речь ---

// «Что нового?» разбиралось как look, хотя игрок говорил это вслух. Кавычки
// снимают двусмысленность и не стоят ни одного вызова модели.
func TestSpeechRecognisedByMarker(t *testing.T) {
	cases := map[string]string{
		`"Что нового?"`:       "Что нового?",
		`«Что нового?»`:       "Что нового?",
		`— Что нового?`:       "Что нового?",
		`- Что нового?`:       "Что нового?",
		`'Что нового?'`:       "Что нового?",
		`«Берн, что слышно?»`: "Берн, что слышно?",
	}
	for line, want := range cases {
		cmd, err := Parse(line)
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		if cmd.Kind != CmdAction || cmd.Intent.Verb != "say" {
			t.Errorf("%q -> вид %v глагол %q, ожидалось действие say", line, cmd.Kind, cmd.Intent.Verb)
			continue
		}
		if cmd.Intent.Args.Text != want {
			t.Errorf("%q -> сказано %q, ожидалось %q", line, cmd.Intent.Args.Text, want)
		}
	}
}

func TestCommandsAreNotMistakenForSpeech(t *testing.T) {
	for _, line := range []string{"look", "question ivar ledger", "facts", "quit"} {
		cmd, err := Parse(line)
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		if cmd.Intent.Verb == "say" {
			t.Errorf("команда %q принята за прямую речь", line)
		}
	}
}

func TestEmptySpeechIsNotSpeech(t *testing.T) {
	for _, line := range []string{`""`, `«»`, `—`, `- `} {
		if said, _, ok := Speech(line); ok {
			t.Errorf("%q принято за речь: %q", line, said)
		}
	}
}

// Кавычки не обязаны стоять в начале: «Обращаясь к Нильсу — "а ты ничего не
// видел?"» это обычная запись, и остаток строки несёт адресата.
func TestSpeechFoundInsideLineWithRemainder(t *testing.T) {
	cases := []struct{ line, said, rest string }{
		{`Обращаясь к Нильсу - "А ты ничего не видел?"`, "А ты ничего не видел?", "Обращаясь к Нильсу -"},
		{`Берну: «что слышно?»`, "что слышно?", "Берну:"},
		{`говорю "привет" и жду`, "привет", "говорю и жду"},
	}
	for _, c := range cases {
		said, rest, ok := Speech(c.line)
		if !ok {
			t.Errorf("%q не распознано как речь", c.line)
			continue
		}
		if said != c.said {
			t.Errorf("%q -> сказано %q, ожидалось %q", c.line, said, c.said)
		}
		if strings.TrimSpace(rest) != strings.TrimSpace(c.rest) {
			t.Errorf("%q -> остаток %q, ожидался %q", c.line, rest, c.rest)
		}
	}
}
