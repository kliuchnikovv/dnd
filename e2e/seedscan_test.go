package e2e

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cli"
)

func runCase(t *testing.T, casePath, scriptPath string, seed int64) string {
	t.Helper()
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	g := gameFor(t, casePath, seed)
	if err := cli.NewSession(g, bytes.NewReader(script), &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	return out.String()
}

// Рукописное дело разрешимо по построению, и это должно означать «при любой
// кости», а не «при удачной». Один провал перехода когда-то рассинхронизировал
// весь скрипт, и поймать такое можно только перебором.
func TestWalkthroughsDoNotDependOnTheDie(t *testing.T) {
	for _, c := range []struct{ name, cs, sc string }{
		{"harbour", "../cases/harbour/case.json", "../cases/harbour/walkthrough.txt"},
		{"forte", "../cases/forte_merlo/case.json", "../cases/forte_merlo/walkthrough.txt"},
	} {
		var bad []int64
		for s := int64(0); s < 40; s++ {
			if !strings.Contains(runCase(t, c.cs, c.sc, s), "Обвинение верно") {
				bad = append(bad, s)
			}
		}
		if len(bad) > 0 {
			t.Errorf("%s: прохождение сорвалось на %d seed из 40: %v", c.name, len(bad), bad)
		}
	}
}
