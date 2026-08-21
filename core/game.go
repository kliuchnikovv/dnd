package core

import (
	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// TokenGrant связывает токен слота обвинения с фактом, который его открывает.
type TokenGrant struct {
	Slot  string // "who" | "how" | "when" | "why"
	Token store.Token
	Fact  store.FactID
}

// party — идентификатор парти. Одиночная игра всё равно им пользуется:
// отношенческий слой дневника ключуется парти с первого дня, чтобы кооп не
// требовал миграции.
func party(id string) string {
	if id == "" {
		return "party"
	}
	return id
}

type Config struct {
	DB *store.DB
	// Party — чья это игра. Пусто означает одиночную парти по умолчанию.
	Party   string
	Rules   RuleSystem
	Dice    Dice
	Truth   accusation.Truth
	Flavour map[string]string
	Tokens  []TokenGrant
	Start   store.NodeID
	Actor   store.CharacterID
	// Aftermath — что стало с виновным и с посёлком; печатается после речи
	// игрока. ColdCase — текст висяка.
	Aftermath string
	ColdCase  string
	// Companion — сущность-напарник; Hints — реплики по фактам.
	Companion store.EntityID
	Hints     map[store.FactID]string
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

	// D — дневники сущностей. Расположение живёт здесь и только здесь:
	// держать его ещё и в Game значило бы иметь два места правды.
	D     *Dossiers
	Debts map[store.EntityID]int

	// Companion — напарник, через которого приходят диегетические подсказки.
	// Hints — авторские реплики, по одной на факт: подсказка указывает на
	// цель, а не на ответ, и придумать её движок не может.
	Companion store.EntityID
	Hints     map[store.FactID]string

	// Theories — гипотезы, зафиксированные глаголом theorize. Ядро их не
	// оценивает и не тратит на них ход: это заметки игрока, а не факты.
	Theories []string

	truth     accusation.Truth
	tokens    []TokenGrant
	flavour   map[string]string
	aftermath string
	coldCase  string

	unlocked map[string]bool
	solved   bool
	dry      int
	hinted   map[store.FactID]bool

	// tool — инструмент, взятый в руки ходом, и узел, где это случилось.
	// Инструмент не уезжает: фонарь со склада не светит в конторе гильдии.
	tool     store.PropID
	toolNode store.NodeID
}

func NewGame(cfg Config) *Game {
	return &Game{
		DB: cfg.DB, Rules: cfg.Rules, Dice: cfg.Dice,
		K: NewKnowledge(cfg.DB), C: NewClocks(cfg.DB),
		Node: cfg.Start, Actor: cfg.Actor,
		D:     NewDossiers(cfg.DB, party(cfg.Party)),
		Debts: map[store.EntityID]int{},
		truth: cfg.Truth, tokens: cfg.Tokens, flavour: cfg.Flavour,
		aftermath: cfg.Aftermath, coldCase: cfg.ColdCase,
		unlocked:  map[string]bool{},
		hinted:    map[store.FactID]bool{},
		Companion: cfg.Companion, Hints: cfg.Hints,
	}
}

// Aftermath — последствия верного обвинения. ColdCase — текст висяка.
func (g *Game) Aftermath() string { return g.aftermath }
func (g *Game) ColdCase() string  { return g.coldCase }

// Solved сообщает, что дело закрыто верным обвинением.
func (g *Game) Solved() bool { return g.solved }

// Flavour отдаёт текст по ключу. Отсутствующий ключ виден в выводе как
// [ключ]: пустая строка на его месте прячет дыру в деле до самого демо.
func (g *Game) Flavour(key string) string {
	if s, ok := g.flavour[key]; ok {
		return s
	}
	return "[" + key + "]"
}
