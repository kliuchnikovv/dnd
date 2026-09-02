package core

import (
	"sort"

	"github.com/kliuchnikovv/dnd/core/scenarios/deduction/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

type AccusationResult struct {
	Refused bool
	Refusal string
	Correct bool
	Attempt int
	Fired   []store.Consequence
	// Summation — развязка: клаузы фактов, чьи токены игрок поставил в слоты,
	// в порядке who → how → when → why. Пуста при ошибочном обвинении.
	Summation []string
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
	if res.Correct {
		res.Summation = g.summation(slots)
		g.solved = true
	}
	fired := g.C.TickAll(1)
	g.applyConsequences(fired)
	res.Fired = fired
	return res
}

// summation собирает речь из клауз тех фактов, что стоят за поставленными
// токенами. Порядок слотов фиксирован: who → how → when → why. Клауза берётся
// только у факта, который игрок фактически положил в слот, — поэтому два
// игрока, пришедшие к верному ответу разными путями, читают разные развязки.
func (g *Game) summation(slots map[string]store.Token) []string {
	var out []string
	said := map[string]bool{}
	for _, slot := range []string{"who", "how", "when", "why"} {
		fact, ok := g.factBehind(slot, slots[slot])
		if !ok {
			continue
		}
		// Один факт может закрывать несколько слотов; повторённая строка
		// читается как сбой, а не как вывод.
		clause := g.DB.Facts[fact].SummationClause
		if clause == "" || said[clause] {
			continue
		}
		said[clause] = true
		out = append(out, clause)
	}
	return out
}

// factBehind находит факт, открывший токен в этом слоте. Берётся первый
// собранный: токен может опираться на несколько фактов, но в речь идёт тот,
// который у парти действительно есть.
func (g *Game) factBehind(slot string, tok store.Token) (store.FactID, bool) {
	for _, t := range g.tokens {
		if t.Slot == slot && t.Token == tok && g.K.Knows(t.Fact) {
			return t.Fact, true
		}
	}
	return "", false
}

func (g *Game) tokenAvailable(slot string, tok store.Token) bool {
	for _, t := range g.AvailableTokens(slot) {
		if t == tok {
			return true
		}
	}
	return false
}
