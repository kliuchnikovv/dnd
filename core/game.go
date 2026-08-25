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
	// CaseID — какое это дело. Ключует канон мира: деталь, решённую в одном
	// деле, нельзя молча применять к другому.
	CaseID store.CaseID
	// Party — чья это игра. Пусто означает одиночную парти по умолчанию.
	Party   string
	Rules   RuleSystem
	Dice    Dice
	Truth   accusation.Truth
	Flavour map[string]string
	// Setting — сеттинг-библия дела: место, время, уклад, погода, то, что
	// «все и так знают». Домен её не читает: это материал для слоя над ним.
	Setting string
	Tokens  []TokenGrant
	Start   store.NodeID
	Actor   store.CharacterID
	// Aftermath — что стало с виновным и с посёлком; печатается после речи
	// игрока. ColdCase — текст висяка.
	Aftermath string
	ColdCase  string
	// Briefing — с чем игрока прислали: кто он, что случилось, чего от него
	// ждут. Hints — реплики по фактам.
	Briefing string
	Hints    map[store.FactID]string
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

	// party — чья это игра. Нужна инвентарю и дневникам: и то и другое
	// принадлежит парти, а не персонажу.
	party string

	// D — дневники сущностей. Расположение живёт здесь и только здесь:
	// держать его ещё и в Game значило бы иметь два места правды.
	D     *Dossiers
	Debts map[store.EntityID]int

	// Briefing — авторское введение в дело. Домен его не читает: печатает
	// его слой над ядром, и лежит оно здесь по той же причине, что и Setting —
	// другого места, где дело собрано целиком, нет.
	Briefing string

	// Hints — авторские реплики, по одной на факт: подсказка указывает на
	// цель, а не на ответ, и придумать её движок не может. Говорящего у неё
	// нет: чутьё принадлежит игроку, а не персонажу (см. hunch.go).
	Hints map[store.FactID]string

	// CaseID — какое это дело. Ключ канона мира.
	CaseID store.CaseID

	// Setting — сеттинг-библия дела. Домен ею не пользуется; она лежит здесь,
	// потому что читать её должен слой над доменом, а другого места, где
	// собрано дело целиком, нет.
	Setting string

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
	// visited — узлы, где игрок УЖЕ БЫЛ. Не смежные: «туда можно дойти» и
	// «ты там был» — разные вещи, и чутьё вправе опираться только на второе.
	visited map[store.NodeID]bool

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
		party: party(cfg.Party),
		D:     NewDossiers(cfg.DB, party(cfg.Party)),
		Debts: map[store.EntityID]int{},
		truth: cfg.Truth, tokens: cfg.Tokens, flavour: cfg.Flavour,
		aftermath: cfg.Aftermath, coldCase: cfg.ColdCase,
		Setting:  cfg.Setting,
		CaseID:   cfg.CaseID,
		unlocked: map[string]bool{},
		hinted:   map[store.FactID]bool{},
		Briefing: cfg.Briefing, Hints: cfg.Hints,
		// Стартовый узел посещён с самого начала: игрок в нём стоит, и
		// подсказка про его держателей законна с первого хода.
		visited: map[store.NodeID]bool{cfg.Start: true},
	}
}

// MoveTo переставляет игрока в узел и запоминает, что он там был.
//
// Ядро само не перемещает: узел меняет слой над ним, когда бросок это
// разрешил. Но ЗАПОМНИТЬ посещение обязано ядро — на посещённое опирается
// чутьё, и оставить эту память вызывающему значило бы иметь два места правды
// о том, где игрок побывал.
func (g *Game) MoveTo(n store.NodeID) {
	g.Node = n
	g.visited[n] = true
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
