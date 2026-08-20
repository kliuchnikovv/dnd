package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDotenvLine(t *testing.T) {
	cases := []struct {
		line, key, value string
		ok               bool
	}{
		{`KEY=value`, "KEY", "value", true},
		{`export KEY=value`, "KEY", "value", true},
		{`KEY="value"`, "KEY", "value", true},
		{`KEY='value'`, "KEY", "value", true},
		{`  KEY = value  `, "KEY", "value", true},
		{`KEY=sk-or-v1-abc=def`, "KEY", "sk-or-v1-abc=def", true},
		{`# комментарий`, "", "", false},
		{``, "", "", false},
		{`мусор без равенства`, "", "", false},
		{`=value`, "", "", false},
	}
	for _, c := range cases {
		k, v, ok := parseDotenvLine(c.line)
		if ok != c.ok || k != c.key || v != c.value {
			t.Errorf("%q -> (%q, %q, %v), ожидалось (%q, %q, %v)",
				c.line, k, v, ok, c.key, c.value, c.ok)
		}
	}
}

func TestLoadDotenvDoesNotOverrideEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FROM_FILE=file\nALREADY_SET=file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALREADY_SET", "environment")

	loadDotenv(path)

	if got := os.Getenv("FROM_FILE"); got != "file" {
		t.Errorf("FROM_FILE = %q, ожидалось из файла", got)
	}
	// Экспортированный ключ обязан перебивать забытый в файле.
	if got := os.Getenv("ALREADY_SET"); got != "environment" {
		t.Errorf("ALREADY_SET = %q — файл перебил окружение", got)
	}
}

func TestLoadDotenvMissingFileIsNotAnError(t *testing.T) {
	loadDotenv(filepath.Join(t.TempDir(), "нет-такого"))
}
