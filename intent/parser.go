package intent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/store"
)

// Result — исход разбора. Ровно одно из трёх: действие, уточняющий вопрос
// или признание, что словарь такого не покрывает.
type Result struct {
	Intent *core.Intent
	// Clarify — вопрос игроку. Парсер договаривается на границе словаря,
	// а не отбивает ввод: отказ без предложения и есть та клетка, которой
	// боятся в закрытом словаре.
	Clarify string
	// Candidate — ввод, не покрытый словарём. Кандидат в новый глагол.
	Candidate string
	// Class — класс предложенного глагола, пуст если глагол не предлагался.
	Class core.VerbClass
}

func (r Result) Accepted() bool { return r.Intent != nil }

type Parser struct {
	gw      *llm.Gateway
	metrics *Metrics
}

func NewParser(gw *llm.Gateway) *Parser {
	return &Parser{gw: gw, metrics: NewMetrics()}
}

func (p *Parser) Metrics() *Metrics { return p.metrics }

const systemPrompt = `Ты переводишь фразу игрока в действие настольной игры.

Правила, которые нельзя нарушать:
1. Глагол выбирается ТОЛЬКО из перечисленных в схеме. Своих не придумывай.
2. target, topic и node — это идентификаторы ИЗ СПИСКА сцены ниже. Ничего,
   чего в списке нет, использовать нельзя: такой сущности в мире не существует.
3. Спросить можно только о теме из списка известных. Если игрок спрашивает о
   том, чего парти не знает, это не действие — верни clarify.
4. Если фраза не ложится ни на один глагол, верни unsupported и опиши в reason,
   чего не хватило. Не подгоняй фразу под неподходящий глагол.
5. Если фраза ложится, но непонятно на что именно, верни clarify с коротким
   вопросом — в характере мира, не служебным языком.

Ты не решаешь, удалось ли действие. Ты только переводишь.`

// Parse переводит текст в интент. Возвращает ошибку только на отказе шлюза
// или сломанном ответе; непонятый ввод — это Result, а не ошибка.
func (p *Parser) Parse(ctx context.Context, text string, hint SceneHint, req llm.Request) (Result, error) {
	req.Role = llm.RoleIntentParser
	req.Schema = SchemaJSON()
	req.System = systemPrompt
	req.Input = "Сцена:\n" + hint.Render() + "\nИгрок пишет: " + text

	resp, err := p.gw.Do(ctx, req)
	if err != nil {
		return Result{}, err
	}

	var raw reply
	if err := json.Unmarshal([]byte(resp.Text), &raw); err != nil {
		return Result{}, fmt.Errorf("intent: ответ не разобрался: %w", err)
	}
	return p.validate(raw, hint), nil
}

// validate — вторая половина гарантии. Схема отвечает за форму ответа, эта
// функция за его смысл: ссылка на сущность вне сцены отклоняется, даже если
// формально валидна.
func (p *Parser) validate(raw reply, hint SceneHint) Result {
	switch raw.Outcome {
	case OutcomeClarify:
		p.metrics.Observe("", false)
		return Result{Clarify: fallback(raw.Clarify, "уточни, что именно ты делаешь")}
	case OutcomeUnsupported:
		p.metrics.Observe("", false)
		return Result{Candidate: fallback(raw.Reason, "словарь такого не покрывает")}
	case OutcomeIntent:
	default:
		p.metrics.Observe("", false)
		return Result{Clarify: "не разобрал, повтори иначе"}
	}

	def, ok := core.LookupVerb(raw.Verb)
	if !ok {
		// Глагола нет в реестре — класс неизвестен, значит это не отказ
		// конкретного класса, а промах модели по схеме.
		p.metrics.Observe("", false)
		return Result{Clarify: "не разобрал, повтори иначе"}
	}

	reject := func(msg string) Result {
		p.metrics.Observe(def.Class, false)
		return Result{Clarify: msg, Class: def.Class}
	}

	if raw.Target != "" && !hint.hasEntity(raw.Target) {
		return reject("этого здесь нет — кого ты имеешь в виду?")
	}
	if raw.Topic != "" && !hint.hasTopic(raw.Topic) {
		return reject("парти об этом ещё ничего не знает")
	}
	if raw.Node != "" && !hint.hasNode(raw.Node) {
		return reject("туда отсюда не пройти")
	}
	for _, f := range raw.Facts {
		if !hint.hasTopic(f) {
			return reject("такого факта парти не знает")
		}
	}

	in := &core.Intent{Verb: def.Verb, Args: core.Args{
		Target:  store.EntityID(raw.Target),
		Topic:   store.FactID(raw.Topic),
		Node:    store.NodeID(raw.Node),
		Item:    raw.Item,
		Ability: raw.Ability,
		Text:    raw.Text,
	}}
	for _, f := range raw.Facts {
		in.Args.Facts = append(in.Args.Facts, store.FactID(f))
	}

	p.metrics.Observe(def.Class, true)
	return Result{Intent: in, Class: def.Class}
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// GameInterpreter связывает парсер с идущей игрой: подсказка собирается на
// каждый ввод, потому что сцена меняется. Реализует интерфейс переводчика,
// которого ждёт CLI, — структурно, без импорта презентации.
type GameInterpreter struct {
	Parser *Parser
	Game   *core.Game
	// Req несёт идентификаторы плательщика и ход; потолки шлюза считаются
	// по ним.
	Req llm.Request
}

func (gi *GameInterpreter) Interpret(ctx context.Context, text string) (*core.Intent, string, error) {
	res, err := gi.Parser.Parse(ctx, text, BuildHint(gi.Game), gi.Req)
	if err != nil {
		return nil, "", err
	}
	switch {
	case res.Accepted():
		return res.Intent, "", nil
	case res.Candidate != "":
		// Ввод вне словаря — это не ошибка, а сигнал о его узости. Игроку
		// говорим честно, метрика уже записана.
		return nil, "так не получится: " + res.Candidate, nil
	default:
		return nil, res.Clarify, nil
	}
}
