package vignette

import (
	"strings"
	"testing"
)

// Страж-редактор держит правду и вычитает её утечку из черновика Мастера
// (ADR-0008). Анти-лик и так конструкцией (Мастеру не дают правду), но страж —
// бэкстоп против дословного эха защищённого факта (напр. инъекция-payload).

func TestKeywordGuardRedactsProtected(t *testing.T) {
	g := KeywordGuard{}
	protected := []string{"это упырь, взявший голос", "оно уйдёт с рассветом"}
	draft := "Голос знаком. И вообще это упырь, взявший голос — берегись."
	res := g.Check(draft, protected, nil, nil)
	if strings.Contains(res.Clean, "это упырь, взявший голос") {
		t.Fatalf("страж не вырезал защищённый факт: %q", res.Clean)
	}
	if len(res.Flagged) == 0 || !res.Changed {
		t.Fatalf("утечка не флагнута: %+v", res)
	}
	if !strings.Contains(res.Clean, "Голос знаком") {
		t.Fatalf("страж срезал живую краску: %q", res.Clean)
	}
}

// ended-карваут: раскрытая развязка (в allowed) НЕ режется, даже если пересекается
// с защищённым.
func TestKeywordGuardKeepsAllowedFinale(t *testing.T) {
	g := KeywordGuard{}
	protected := []string{"оно уйдёт с рассветом"}
	allowed := []string{"Рассвет — оно уходит с рассветом, ты достоял."}
	draft := "Рассвет — оно уходит с рассветом, ты достоял."
	res := g.Check(draft, protected, nil, allowed)
	if !strings.Contains(res.Clean, "ты достоял") {
		t.Fatalf("страж срезал разрешённую развязку: %q", res.Clean)
	}
}

// Нет утечки — черновик проходит дословно.
func TestKeywordGuardPassesClean(t *testing.T) {
	g := KeywordGuard{}
	res := g.Check("Буря воет за ставнями, пёс рычит.", []string{"это упырь"}, nil, nil)
	if res.Changed || len(res.Flagged) != 0 {
		t.Fatalf("чистый черновик тронут: %+v", res)
	}
}
