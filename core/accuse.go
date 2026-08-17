package core

import (
	"sort"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

type AccusationResult struct {
	Refused bool
	Refusal string
	Correct bool
	Attempt int
	Fired   []store.Consequence
}

// AvailableTokens возвращает токены слота, подкреплённые собранными фактами.
func (g *Game) AvailableTokens(slot string) []store.Token {
	var out []store.Token
	seen := map[store.Token]bool{}
	for _, t := range g.tokens {
		if t.Slot != slot || seen[t.Token] || !g.K.Knows(t.Fact) {
			continue
		}
		seen[t.Token] = true
		out = append(out, t.Token)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Accuse проверяет форму целиком. Ошибочная форма не сообщает, какой слот
// неверен, и стоит тика часов — иначе слоты брутфорсятся по одному.
func (g *Game) Accuse(f accusation.Form) AccusationResult {
	if !f.Complete() {
		return AccusationResult{Refused: true, Refusal: "форма заполнена не полностью"}
	}
	slots := map[string]store.Token{"who": f.Who, "how": f.How, "when": f.When, "why": f.Why}
	for slot, tok := range slots {
		if !g.tokenAvailable(slot, tok) {
			return AccusationResult{Refused: true,
				Refusal: "токен «" + string(tok) + "» не подкреплён собранным фактом"}
		}
	}

	g.Attempts++
	res := AccusationResult{Attempt: g.Attempts, Correct: g.truth.Check(f)}
	fired := g.C.TickAll(1)
	g.applyConsequences(fired)
	res.Fired = fired
	return res
}

func (g *Game) tokenAvailable(slot string, tok store.Token) bool {
	for _, t := range g.AvailableTokens(slot) {
		if t == tok {
			return true
		}
	}
	return false
}
