package core

import (
	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// TokenGrant связывает токен слота обвинения с фактом, который его открывает.
type TokenGrant struct {
	Slot  string      // "who" | "how" | "when" | "why"
	Token store.Token
	Fact  store.FactID
}

type Config struct {
	DB      *store.DB
	Rules   RuleSystem
	Dice    Dice
	Truth   accusation.Truth
	Flavour map[string]string
	Tokens  []TokenGrant
	Start   store.NodeID
	Actor   store.CharacterID
}

// Game — всё изменяемое состояние прогона. Ядро; системы правил здесь нет
// нигде, кроме поля Rules за интерфейсом.
type Game struct {
	DB    *store.DB
	Rules RuleSystem
	Dice  Dice
	K     *Knowledge
	C     *Clocks

	Node     store.NodeID
	Actor    store.CharacterID
	Attempts int
	Detected bool

	Disposition map[store.EntityID]int
	Debts       map[store.EntityID]int

	truth   accusation.Truth
	tokens  []TokenGrant
	flavour map[string]string

	unlocked map[string]bool
}

func NewGame(cfg Config) *Game {
	return &Game{
		DB: cfg.DB, Rules: cfg.Rules, Dice: cfg.Dice,
		K: NewKnowledge(cfg.DB), C: NewClocks(cfg.DB),
		Node: cfg.Start, Actor: cfg.Actor,
		Disposition: map[store.EntityID]int{},
		Debts:       map[store.EntityID]int{},
		truth:       cfg.Truth, tokens: cfg.Tokens, flavour: cfg.Flavour,
		unlocked:    map[string]bool{},
	}
}

// Flavour отдаёт текст по ключу. Отсутствующий ключ виден в выводе как
// [ключ]: пустая строка на его месте прячет дыру в деле до самого демо.
func (g *Game) Flavour(key string) string {
	if s, ok := g.flavour[key]; ok {
		return s
	}
	return "[" + key + "]"
}
