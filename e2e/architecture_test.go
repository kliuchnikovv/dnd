package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestCoreMentionsNoDiceVocabulary(t *testing.T) {
	// Ядро не должно знать словарь системы правил даже на уровне строк.
	forbidden := []string{"d20", "grit +", "порог 14", "attribute"}
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

func TestNoLLMAndNoNetworkAnywhere(t *testing.T) {
	forbidden := []string{
		`"net/http"`, `"net"`, "anthropic", "openai", "completion(",
	}
	// architectureTestFile содержит сам список запрещённых строк как данные —
	// без самоисключения тест валился бы на собственном исходнике всегда,
	// независимо от состояния остального дерева.
	const architectureTestFile = "architecture_test.go"
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "docs" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if filepath.Base(path) == architectureTestFile {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, f := range forbidden {
			if strings.Contains(strings.ToLower(string(body)), f) {
				t.Errorf("%s содержит %q — в M1a этого быть не может", path, f)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNoExternalDependencies(t *testing.T) {
	body, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "require") {
		t.Errorf("появились внешние зависимости:\n%s", body)
	}
}
