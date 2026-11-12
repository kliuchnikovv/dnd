package dotenv

// Тесты загрузчика .env. Главное, что тут проверяется — ПРИОРИТЕТ:
// переменная, уже заданная в окружении, НЕ перетирается файлом. Это то, что
// даёт возможность переопределить модель на CI одним `MODEL=... go run`,
// не редактируя .env.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLine(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		key   string
		value string
		ok    bool
	}{
		{"просто", "KEY=value", "KEY", "value", true},
		{"с export", "export KEY=value", "KEY", "value", true},
		{"в двойных кавычках", `KEY="va lue"`, "KEY", "va lue", true},
		{"в одинарных кавычках", `KEY='va lue'`, "KEY", "va lue", true},
		{"с пробелами", "  KEY  =  value  ", "KEY", "value", true},
		{"пустое значение", "KEY=", "KEY", "", true},
		{"знак равенства в значении", "KEY=a=b", "KEY", "a=b", true},
		{"комментарий", "# KEY=value", "", "", false},
		{"пустая строка", "   ", "", "", false},
		{"нет знака равенства", "KEY value", "", "", false},
		{"нет ключа", "=value", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			k, v, ok := ParseLine(c.line)
			if ok != c.ok || k != c.key || v != c.value {
				t.Errorf("ParseLine(%q) = (%q,%q,%v), ждали (%q,%q,%v)",
					c.line, k, v, ok, c.key, c.value, c.ok)
			}
		})
	}
}

func TestLoadSetsVars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("EVAL_TEST_A=one\nexport EVAL_TEST_B=\"two\"\n# c\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Unsetenv("EVAL_TEST_A")
		os.Unsetenv("EVAL_TEST_B")
	})

	Load(path)

	if got := os.Getenv("EVAL_TEST_A"); got != "one" {
		t.Errorf("EVAL_TEST_A = %q, ждали one", got)
	}
	if got := os.Getenv("EVAL_TEST_B"); got != "two" {
		t.Errorf("EVAL_TEST_B = %q, ждали two", got)
	}
}

func TestLoadDoesNotOverrideExistingEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("EVAL_TEST_C=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EVAL_TEST_C", "from-env")
	t.Cleanup(func() { os.Unsetenv("EVAL_TEST_C") })

	Load(path)

	if got := os.Getenv("EVAL_TEST_C"); got != "from-env" {
		t.Errorf("окружение перебито файлом: %q, ждали from-env", got)
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	// Отсутствие .env — норма: инструмент должен работать офлайн и ругаться
	// на отсутствие ключа сам, а не на отсутствие файла.
	Load(filepath.Join(t.TempDir(), "нет-такого-файла"))
}
