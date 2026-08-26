package llm

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// Реестр — единственная правда о полномочиях (ADR-0001). Правда, полная не
// целиком, хуже устной договорённости: роль без записи молча получает
// пустые полномочия, а вывод строгости — пустой ответ. Полнота проверяется
// по AllRoles, который сам проверен на полноту в role_test.go.
func TestCapabilitiesCoverEveryRole(t *testing.T) {
	for _, role := range AllRoles() {
		c, ok := Capabilities[role]
		if !ok {
			t.Errorf("роль %q объявлена, но капабилити у неё нет: "+
				"полномочия по умолчанию — это дыра, а не значение", role)
			continue
		}
		if len(c.Reads) == 0 {
			t.Errorf("роль %q ничего не читает — так не бывает ни у одной", role)
		}
	}
	for role := range Capabilities {
		if !listedInAllRoles(role) {
			t.Errorf("капабилити объявлены для %q, которой в AllRoles нет", role)
		}
	}
}

func listedInAllRoles(r Role) bool {
	for _, x := range AllRoles() {
		if x == r {
			return true
		}
	}
	return false
}

// Правда дела и чужие гейтнутые факты вне scope ЛЮБОЙ роли. Проверять это
// перечислением записей бессмысленно: запись добавят вместе с константой.
// Поэтому стережём сам набор объявленных константа-скоупов — запретного
// домена чтения просто не существует, и завести его нельзя незаметно.
func TestNoForbiddenReadScopeDeclared(t *testing.T) {
	scopes := declaredConsts(t, "capability.go", "ReadScope")
	if len(scopes) == 0 {
		t.Fatal("в capability.go не нашлось ни одной константы ReadScope — сломан разбор")
	}
	forbidden := []string{"truth", "allholders", "everyholder", "secret"}
	for name, value := range scopes {
		for _, f := range forbidden {
			if strings.Contains(strings.ToLower(name), f) ||
				strings.Contains(strings.ToLower(string(value)), f) {
				t.Errorf("объявлен scope %s (%q): правда дела и гейтнутые чужие факты "+
					"недоступны никому из LLM by-design", name, value)
			}
		}
	}
}

// Ни у одной LLM-роли нет права прямой мутации: она предлагает, применяет
// ядро (ADR-0001). Это держится конструктивно — в Capability нет поля, куда
// такое право можно было бы записать. Тест стережёт, чтобы поле не завели.
func TestCapabilityHasNoDirectMutationField(t *testing.T) {
	fields := structFields(t, "capability.go", "Capability")
	if len(fields) == 0 {
		t.Fatal("у Capability не нашлось полей — сломан разбор")
	}
	for _, name := range fields {
		lower := strings.ToLower(name)
		for _, f := range []string{"mutate", "apply", "write"} {
			if strings.Contains(lower, f) {
				t.Errorf("поле Capability.%s означает прямую мутацию: "+
					"LLM предлагают, ядро располагает", name)
			}
		}
	}
}

// declaredConsts — константы названного типа из файла пакета: имя к значению.
// Общий для role_test.go и этого файла: второй разборщик отстал бы так же,
// как отстал бы второй список ролей.
func declaredConsts(t *testing.T, file, typeName string) map[string]string {
	t.Helper()
	f := parsePackageFile(t, file)
	out := map[string]string{}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			// Тип пишется у первой константы группы и наследуется дальше;
			// здесь каждая объявлена со своим типом, поэтому проверяем свой.
			id, ok := vs.Type.(*ast.Ident)
			if !ok || id.Name != typeName {
				continue
			}
			for i, name := range vs.Names {
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				out[name.Name] = lit.Value[1 : len(lit.Value)-1]
			}
		}
	}
	return out
}

// structFields — имена полей названной структуры из файла пакета.
func structFields(t *testing.T, file, typeName string) []string {
	t.Helper()
	f := parsePackageFile(t, file)
	var out []string
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != typeName {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					out = append(out, name.Name)
				}
			}
		}
	}
	return out
}

func parsePackageFile(t *testing.T, file string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("разбор %s: %v", file, err)
	}
	return f
}

// Роль озвучки вариантов ничего не предлагает ядру, и из этого выводится, что
// строгий провайдер ей не нужен: её выход не доходит до состояния, и плохой
// разбор стоит бледной строки, а не канона.
func TestOptionsRoleProposesNothing(t *testing.T) {
	cap, ok := Capabilities[RoleOptions]
	if !ok {
		t.Fatal("у роли озвучки вариантов нет капабилити")
	}
	if len(cap.Proposes) != 0 {
		t.Errorf("роль озвучки что-то предлагает ядру: %v", cap.Proposes)
	}
	if RequiresStrictOutput(RoleOptions) {
		t.Error("роли озвучки навязан строгий провайдер")
	}
}

// Скоуп ровно тот, из чего строится набор: сцена, знание парти, носимое.
// Правды дела в наборе ReadScope нет и быть не может по конструкции.
func TestOptionsRoleReadsOnlyWhatTheSetIsBuiltFrom(t *testing.T) {
	want := map[ReadScope]bool{
		ReadScenePublic: true, ReadPartyKnowledge: true, ReadCarriedItems: true,
	}
	for _, r := range Capabilities[RoleOptions].Reads {
		if !want[r] {
			t.Errorf("роль озвучки читает лишнее: %q", r)
		}
		delete(want, r)
	}
	for r := range want {
		t.Errorf("роль озвучки не читает %q — набор без этого не назвать словами", r)
	}
}
