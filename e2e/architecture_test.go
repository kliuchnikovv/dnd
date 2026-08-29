package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// pureLayers — слои, которые обязаны остаться свободными от сети и моделей.
// Это и есть шов: чистым обязано быть ЯДРО, а не весь репозиторий. Пакет llm
// и презентационный слой моделям звонить вправе — домен нет.
var pureLayers = []string{"../core/...", "../rules/...", "../store/...", "../dice/...", "../cases"}

func TestCoreDoesNotDependOnRules(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "../core").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, forbidden := range []string{"/rules/", "math/rand"} {
		if strings.Contains(string(out), forbidden) {
			t.Errorf("core тянет %q — шов протёк", forbidden)
		}
	}
}

// TestPureLayersDoNotDependOnLLM — домен не имеет права знать о моделях даже
// транзитивно. Слайс M2 добавляет пакет llm; этот тест держит границу.
func TestPureLayersDoNotDependOnLLM(t *testing.T) {
	for _, layer := range pureLayers {
		out, err := exec.Command("go", "list", "-deps", layer).Output()
		if err != nil {
			t.Fatalf("go list %s: %v", layer, err)
		}
		for _, forbidden := range []string{"/llm", "net/http"} {
			if strings.Contains(string(out), forbidden) {
				t.Errorf("%s тянет %q — домен не должен знать о моделях и сети",
					layer, forbidden)
			}
		}
	}
}

func TestCoreMentionsNoDiceVocabulary(t *testing.T) {
	forbidden := []string{"d20", "grit +", "attribute"}
	err := filepath.Walk("../core", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, f := range forbidden {
			if strings.Contains(strings.ToLower(string(body)), f) {
				t.Errorf("%s содержит %q", path, f)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestPureLayersNameNoVendor — имена провайдеров не должны просачиваться в
// домен. В пакете llm они законны, поэтому он в список не входит.
func TestPureLayersNameNoVendor(t *testing.T) {
	vendors := []string{"anthropic", "openai", "openrouter", "gemini"}
	for _, layer := range []string{"../core", "../rules", "../store", "../dice", "../cases"} {
		err := filepath.Walk(layer, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, v := range vendors {
				if strings.Contains(strings.ToLower(string(body)), v) {
					t.Errorf("%s называет вендора %q", path, v)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestDependenciesAreAllowlisted — внешние зависимости только из списка.
// Пустой go.mod был инвариантом M1a; в M2 появляется SDK, и правильная
// защита — allowlist, а не запрет любых require.
func TestDependenciesAreAllowlisted(t *testing.T) {
	allowed := []string{
		"github.com/anthropics/anthropic-sdk-go",
		// bubbletea/bubbles/lipgloss — полноэкранный режим tui: он
		// необязателен и лежит НАД игрой, но прокрутка с переносами строк,
		// ширина символов и resize терминала — та мелкая логика, где своя
		// реализация обходится дороже готовой и проверенной. Список
		// расширен сознательно, а не по недосмотру.
		"github.com/charmbracelet/bubbletea",
		"github.com/charmbracelet/bubbles",
		"github.com/charmbracelet/lipgloss",
		// golang.org/x/term — единственный честный способ спросить у файла,
		// терминал он или нет. os.ModeCharDevice отвечает "да" и на
		// /dev/null, потому что это тоже символьное устройство: не-терминал
		// (пайп, -script) уходил в полноэкранную ветку и падал на open
		// /dev/tty. Это официальный модуль самого Go, а не сторонняя libc,
		// и другого источника правды для этого вопроса нет.
		"golang.org/x/term",
		// coder/websocket — транспорт серверного слоя (cmd/server). Ручной
		// RFC 6455 поверх net/http — это фрейминг, маски и ping, где своя
		// реализация обходится дороже готовой; библиотека минимальна и
		// context-aware. Живёт НАД игрой, домена не касается.
		"github.com/coder/websocket",
		// jackc/pgx — драйвер Postgres для долговечного журнала (cmd/server).
		// Журнал и его реплей — опора восстановления сессии (ADR-0002); свой
		// протокол Postgres писать незачем. Живёт НАД игрой, в чистые слои не
		// просачивается.
		"github.com/jackc/pgx/v5",
		// golang-jwt/jwt — токены доступа и refresh для аутентификации (auth пакет).
		// Живёт НАД игрой в слое входа, домена не касается.
		"github.com/golang-jwt/jwt/v5",
		// google.golang.org/api — проверка Google ID-token для аутентификации
		// (auth пакет, NewGoogleVerifier). Живёт НАД игрой в слое входа, домена не касается.
		// Фильтр ниже включает google.golang.org/ для проверки этой и аналогичных зависимостей.
		"google.golang.org/api",
	}
	body, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "github.com/") &&
			!strings.HasPrefix(line, "golang.org/") &&
			!strings.HasPrefix(line, "google.golang.org/") {
			continue
		}
		if strings.HasPrefix(line, "github.com/kliuchnikovv/dnd") {
			continue
		}
		// Транзитивные зависимости — дело SDK, а не наше решение. Инвариант
		// касается того, на что мы подписались сами.
		if strings.Contains(line, "// indirect") {
			continue
		}
		ok := false
		for _, a := range allowed {
			if strings.HasPrefix(line, a) {
				ok = true
			}
		}
		if !ok {
			t.Errorf("незаявленная зависимость: %s", line)
		}
	}
}

// TestCliDoesNotDependOnTUI — направление зависимости лежит НАД cli, а не
// внутри него: tui необязателен ровно потому, что cli о нём не знает. Если
// это когда-нибудь развернётся (например, через общий вспомогательный
// пакет), построчный режим перестанет собираться без bubbletea.
func TestCliDoesNotDependOnTUI(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "../cli").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	if strings.Contains(string(out), "/tui") {
		t.Error("cli тянет tui — направление зависимости развернулось")
	}
}
