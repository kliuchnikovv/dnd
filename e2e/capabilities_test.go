package e2e

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
)

// ADR-0001: у каждой роли свои полномочия, и записаны они в реестре
// капабилити. Роль без записи получает пустые полномочия молча — а молчание
// здесь неотличимо от «ничего не читает и ничего не предлагает».
//
// Та же проверка есть внутри пакета llm. Здесь она стоит второй раз нарочно:
// граница — то место, где инвариант переживает переезд реестра в другой
// пакет и запуск тестов по одному.
func TestEveryRoleHasCapability(t *testing.T) {
	for _, role := range llm.AllRoles() {
		c, ok := llm.Capabilities[role]
		if !ok {
			t.Errorf("роль %q объявлена без капабилити", role)
			continue
		}
		if len(c.Reads) == 0 {
			t.Errorf("роль %q ничего не читает — так не бывает ни у одной", role)
		}
	}
}

// Ни одна LLM-роль не меняет состояние сама: она предлагает, применяет ядро.
// Сегодня это держится тем, что применение мутаций, цены и последствий —
// неэкспортированные методы core: позвать их снаружи нельзя физически.
//
// Тест — заготовка под будущий API мутаций: когда предложения нарратора и
// куратора пойдут в ядро, применять их будет по-прежнему только core, и
// экспортировать применяющий метод придётся через этот тест.
func TestOnlyCoreAppliesMutations(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "../core/mutate.go", nil, 0)
	if err != nil {
		t.Fatalf("разбор core/mutate.go: %v", err)
	}
	var seen int
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		seen++
		if fn.Name.IsExported() {
			t.Errorf("core.%s экспортирован: применение изменений становится "+
				"доступно снаружи ядра, и «LLM предлагают, ядро располагает» "+
				"держится уже только на договорённости", fn.Name.Name)
		}
	}
	if seen == 0 {
		t.Fatal("в core/mutate.go не нашлось функций — сломан разбор")
	}
}
