package e2e

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
	"github.com/kliuchnikovv/dnd/view"
)

// axisVocabulary — слова конкретных ПРАВИЛА и СЦЕНАРИЯ реф-конфигурации. Как
// architecture_test бережёт границу домена, так этот тест бережёт вторую ось:
// вид держит дескрипторы, а называет их правило и сценарий. Появись любое из
// этих слов строковым литералом в пакете view — форма вида срослась бы с одной
// парой, и клиент перестал бы быть безразличен к правилу и сценарию.
//
// Проверяются ИМЕННО литералы, а не байты файла: комментарии вида как раз
// объясняют, каких слов в нём нет («клиент не знает слов „grit"/„casebook"»), и
// грепом по тексту тест ловил бы собственную документацию.
var axisVocabulary = []string{
	// ось правила «Порог»: метки мер и словарь кости
	"ранения", "раны", "grit", "d20",
	// ось сценария-детектива: панель и слоты обвинения
	"casebook", "досье", "who", "how", "when", "why",
}

// TestViewKnowsNoRuleOrScenarioVocabulary — шов агностичности. Строковые
// литералы пакета view не содержат вокабуляра ни правила, ни сценария.
func TestViewKnowsNoRuleOrScenarioVocabulary(t *testing.T) {
	lits := stringLiterals(t, "../view")
	for _, lit := range lits {
		low := strings.ToLower(lit.value)
		for _, word := range axisVocabulary {
			if strings.Contains(low, word) {
				t.Errorf("%s: литерал %q несёт осевое слово %q — вид сросся с парой",
					lit.pos, strconv.Quote(lit.value), word)
			}
		}
	}
}

type literal struct {
	value string
	pos   string
}

// stringLiterals собирает строковые литералы всех не-тестовых файлов пакета.
func stringLiterals(t *testing.T, dir string) []literal {
	t.Helper()
	var out []literal
	fset := token.NewFileSet()
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			v, uerr := strconv.Unquote(lit.Value)
			if uerr != nil {
				return true
			}
			out = append(out, literal{value: v, pos: fset.Position(lit.Pos()).String()})
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestViewCarriesOnlyKnownFacts — leak-охрана. Вид собирается из read-scope:
// известный факт в нём есть, гейтнутого-неизвестного — нет. Правды дела
// (ответа who/how/when/why) во вид попасть тоже неоткуда: её читает только
// Accuse, а сборка вида к ней не ходит.
func TestViewCarriesOnlyKnownFacts(t *testing.T) {
	cfg, err := cases.Load("../cases/harbour/case.json")
	if err != nil {
		t.Fatalf("загрузка дела: %v", err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(1).Stream("resolve")
	g := core.NewGame(*withActorCharacter(cfg))

	tv := view.Build(g, core.TurnResult{}, nil, cli.RefRuleset{}, cli.Detective{}, "")
	data, err := json.Marshal(tv)
	if err != nil {
		t.Fatalf("сериализация вида: %v", err)
	}
	blob := string(data)

	// На старте «Гавани» парти знает ровно один факт — он в виде должен быть.
	if !strings.Contains(blob, "f_body_found") {
		t.Errorf("известный факт не попал в вид: %s", blob)
	}
	// Гейтнутые-неизвестные факты не собраны — и в виде их быть не должно.
	for _, hidden := range []string{"f_toke_lied", "f_shortfall", "f_ivar_alibi", "f_ledger_erasure"} {
		if strings.Contains(blob, hidden) {
			t.Errorf("неизвестный факт %q утёк в вид: %s", hidden, blob)
		}
	}
}
