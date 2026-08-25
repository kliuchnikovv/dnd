package e2e

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Единственная граница состояния (ADR-0001, дизайн API мутаций, инвариант 1).
//
// Состояние мира меняется двумя дверями: Apply — команды игрока,
// ProposeMutation — предложения недоверенных. Обе валидируют. Третья дверь
// опасна не тем, что кто-то напишет плохое значение, а тем, что валидация
// станет договорённостью: код, который ходит в стор напрямую, проверок не
// проходит и о них не знает.
//
// Проверить это исчерпывающе нельзя — Go не даёт запретить вызов метода из
// чужого пакета иначе, чем неэкспортируя его. Поэтому здесь два теста и один
// приём: то, что можно закрыть языком, закрыто языком (canonPut неэкспортирован,
// applyMutations неэкспортирован, TestOnlyCoreAppliesMutations держит список),
// а остальное стережёт закрытый список ниже. Он не запрещает — он делает
// новую дверь видимой: добавить её можно, но только вписав в список и объяснив,
// зачем.

// aboveDomain — слои над доменом. Домен (core, store, rules, cases, dice) в
// список не входит: внутри границы писать в состояние законно, в этом и смысл.
var aboveDomain = []string{"../actor", "../cli", "../tui", "../master",
	"../cmd", "../intent", "../propose"}

// worldWrites — методы, которые меняют состояние мира. Позвать их снаружи
// домена значит обойти обе двери.
//
// Журнала здесь нет намеренно: AppendCommand/AppendAudit/MarkApplied пишут не
// мир, а его историю, и ведёт её по ADR-0002 именно слой над доменом.
var worldWrites = map[string]string{
	"Tick":            "тик часов",
	"TickAll":         "тик всех часов",
	"Adjust":          "сдвиг расположения",
	"Learn":           "выдача факта парти",
	"AddItem":         "выдача предмета",
	"Acquire":         "выдача предмета",
	"MarkPresented":   "отметка предъявленного",
	"CloseThread":     "закрытие нити",
	"ProposeMutation": "предложение мутации мимо капабилити-гейта",
	"KnowPlace":       "выдача знания о месте",
}

// allowed — известные исключения: файл и метод, который ему позволен. Каждая
// строка — обещание, что дверь проверена глазами.
var allowed = map[string]map[string]string{
	// Гейт — единственный законный вызывающий ядра для предложений: в нём и
	// стоит проверка полномочий роли. Прямой вызов из другого места её обошёл
	// бы, и «роль не вправе» держалось бы уже только на договорённости.
	"../propose/propose.go": {"ProposeMutation": "здесь и стоит капабилити-гейт"},
	// Диалоговая память: что персонаж уже рассказал и что было сказано в
	// разговоре. Мутацией это пока не выражено — своего вида в закрытом наборе
	// у неё нет, а придумывать его ради одного вызова значило бы расширить
	// набор раньше, чем понятно, каким он должен быть. Границы дела и правды
	// эта запись не касается: она пишет дневник сущности, не факты.
	"../actor/actor.go": {
		"MarkTold": "диалоговая память: тема помечена рассказанной",
		"Remember": "диалоговая память: реплика легла в дневник сущности",
	},
}

func TestStateIsWrittenOnlyThroughTheBorder(t *testing.T) {
	for _, layer := range aboveDomain {
		err := filepath.Walk(layer, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			// Тесты строят миры руками, и это законно: фикстура — не игровой
			// код, и валидацию ей обходить нечем.
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if perr != nil {
				return perr
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				what, forbidden := worldWrites[sel.Sel.Name]
				if !forbidden || allowed[path][sel.Sel.Name] != "" {
					return true
				}
				t.Errorf("%s: %s — %s мимо границы: третья дверь не проходит "+
					"проверок, которые стоят на первых двух",
					path, sel.Sel.Name, what)
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("обход %s: %v", layer, err)
		}
	}
}

// Таблицы состояния не заполняются снаружи домена: запись в map стора — та же
// третья дверь, только без имени метода.
func TestStateTablesAreNotWrittenAboveDomain(t *testing.T) {
	tables := map[string]bool{
		"Canon": true, "Facts": true, "Holders": true, "Unlocks": true,
		"Characters": true, "Clocks": true, "Entities": true, "Locations": true,
		"Items": true, "Dossiers": true, "Props": true, "Relations": true,
	}
	for _, layer := range aboveDomain {
		err := filepath.Walk(layer, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if perr != nil {
				return perr
			}
			ast.Inspect(file, func(n ast.Node) bool {
				assign, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for _, lhs := range assign.Lhs {
					index, ok := lhs.(*ast.IndexExpr)
					if !ok {
						continue
					}
					sel, ok := index.X.(*ast.SelectorExpr)
					if !ok || !tables[sel.Sel.Name] {
						continue
					}
					t.Errorf("%s: запись в таблицу %s снаружи домена — "+
						"состояние меняется мимо границы", path, sel.Sel.Name)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("обход %s: %v", layer, err)
		}
	}
}
