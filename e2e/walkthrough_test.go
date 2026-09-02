package e2e

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
)

func newGame(t *testing.T, seed int64) *core.Game {
	t.Helper()
	cfg, err := cases.Load("../cases/harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(seed).Stream("resolve")
	return core.NewGame(*withActorCharacter(cfg))
}

func TestWalkthroughReachesCorrectAccusation(t *testing.T) {
	script, err := os.ReadFile("../cases/harbour/walkthrough.txt")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	g := newGame(t, 3)
	if err := cli.NewSession(g, bytes.NewReader(script), &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if !strings.Contains(out.String(), "Обвинение верно") {
		t.Fatalf("прохождение не дошло до верного обвинения:\n%s", out.String())
	}
}

func TestWalkthroughIsReproducible(t *testing.T) {
	script, _ := os.ReadFile("../cases/harbour/walkthrough.txt")
	run := func() string {
		var out bytes.Buffer
		cli.NewSession(newGame(t, 3), bytes.NewReader(script), &out).Run()
		return out.String()
	}
	if run() != run() {
		t.Error("один seed и один скрипт дали разные транскрипты")
	}
}

// Предмет — рычаг, а не единственный ключ. Дело обязано проходиться и без
// предписания: обязательный источник того же факта (Нильс) остаётся на месте,
// иначе решаемость by-design держалась бы на одной бумаге.
func TestWalkthroughPassesWithoutPresentingTheWrit(t *testing.T) {
	script, err := os.ReadFile("../cases/harbour/walkthrough.txt")
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	var dropped int
	for _, line := range strings.Split(string(script), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "present ") {
			dropped++
			continue
		}
		kept = append(kept, line)
	}
	if dropped == 0 {
		t.Fatal("в прохождении нет предъявления — тест перестал проверять то, ради чего написан")
	}

	var out bytes.Buffer
	g := newGame(t, 3)
	if err := cli.NewSession(g, strings.NewReader(strings.Join(kept, "\n")), &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if !strings.Contains(out.String(), "Обвинение верно") {
		t.Fatalf("без предписания дело не проходится:\n%s", out.String())
	}
}

// Предъявление годной бумаги открывает факт без броска и теплит расположение —
// тот жанровый бит, которого не хватало: «у меня бумага, ты обязан говорить».
func TestPresentingTheWritOpensBernsFact(t *testing.T) {
	g := newGame(t, 3)
	before := g.D.Disposition("e_bern")

	// Без предъявления гейт закрыт: расспрос ничего не даёт.
	closed := g.Apply(core.Intent{Verb: "question", Actor: g.Actor,
		Args: core.Args{Target: "e_bern", Topic: "f_toke_at_quay"}})
	if len(closed.Learned) != 0 {
		t.Fatalf("факт выдан без предписания: %+v", closed.Learned)
	}

	got := g.Apply(core.Intent{Verb: "present", Actor: g.Actor,
		Args: core.Args{Target: "e_bern", Item: "i_writ"}})
	if got.Refused {
		t.Fatalf("предъявление отказано: %s", got.Refusal)
	}
	if got.Res != nil {
		t.Error("предъявление бросило кость — годная бумага это не проба")
	}
	if len(got.Learned) != 1 || got.Learned[0].Fact != "f_toke_at_quay" {
		t.Fatalf("факт не открылся: %+v", got.Learned)
	}
	if after := g.D.Disposition("e_bern"); after != before+1 {
		t.Errorf("расположение %d, было %d", after, before)
	}
}

// Дело обязано быть проходимым ВСЛЕПУЮ: игрок не знает идентификаторов фактов
// и добывает всё сам — осмотром и открытыми вопросами. Четыре плейтеста подряд
// провалились именно здесь, и авторское прохождение этого не ловило: его писал
// тот, кто знает все идентификаторы наизусть.
func TestCaseIsSolvableWithoutKnowingFactIDs(t *testing.T) {
	script, err := os.ReadFile("../cases/harbour/blind_path.txt")
	if err != nil {
		t.Fatal(err)
	}
	// Путь вслепую не имеет права называть идентификатор факта где-либо, кроме
	// compare: там оба берутся из вывода команды facts.
	for _, line := range strings.Split(string(script), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "compare") {
			continue
		}
		if strings.Contains(line, "f_") {
			t.Errorf("путь вслепую называет идентификатор факта: %q", line)
		}
	}

	var out bytes.Buffer
	if err := cli.NewSession(newGame(t, 7), bytes.NewReader(script), &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if !strings.Contains(out.String(), "Обвинение верно") {
		t.Fatalf("вслепую дело не проходится:\n%s", out.String())
	}
}
