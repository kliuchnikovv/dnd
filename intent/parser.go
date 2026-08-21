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

Ты не решаешь, удалось ли действие. Ты только переводишь.

Обязательные аргументы по глаголам:
%s
Примеры. Обрати внимание: обязательное поле заполняется ВСЕГДА, даже когда
игрок называет персонажа по имени, а не по идентификатору.

Сцена: e_bern — Берн, стражник
Игрок: «Поздороваться с Берном»
Ответ: {"outcome":"intent","verb":"talk_to","target":"e_bern"}

Сцена: e_ivar — Ивар, кузнец; известные темы: f_ledger — гроссбух
Игрок: «спрошу кузнеца про книгу»
Ответ: {"outcome":"intent","verb":"question","target":"e_ivar","topic":"f_ledger"}

Сцена: e_ivar — Ивар, кузнец; известных тем нет
Игрок: «спрошу кузнеца, кто убийца»
Ответ: {"outcome":"clarify","clarify":"Об этом парти пока ничего не знает."}`

// Parse переводит текст в интент. Возвращает ошибку только на отказе шлюза
// или сломанном ответе; непонятый ввод — это Result, а не ошибка.
func (p *Parser) Parse(ctx context.Context, text string, hint SceneHint, req llm.Request) (Result, error) {
	res, repair, err := p.attempt(ctx, text, hint, req, "")
	if err != nil {
		return Result{}, err
	}
	// Один раунд починки. Слабая модель игнорирует инструкцию об обязательном
	// поле, но исправляется, когда ей называют пропущенное. Второго раунда нет
	// намеренно: дальше это уже не недопонимание, а неподходящий глагол.
	if repair != "" {
		res, _, err = p.attempt(ctx, text, hint, req, repair)
		if err != nil {
			return Result{}, err
		}
	}
	p.observe(res)
	return res, nil
}

func (p *Parser) attempt(ctx context.Context, text string, hint SceneHint,
	req llm.Request, repair string) (Result, string, error) {
	req.Role = llm.RoleIntentParser
	req.Schema = schemaJSONFor(hint)
	req.System = fmt.Sprintf(systemPrompt, arityBrief())
	req.Input = "Сцена:\n" + hint.Render() + "\nИгрок пишет: " + text
	if repair != "" {
		req.Input += "\n\nПредыдущий ответ был неполон: " + repair +
			"\nВерни тот же глагол, заполнив пропущенное значением из сцены."
	}

	resp, err := p.gw.Do(ctx, req)
	if err != nil {
		return Result{}, "", err
	}
	var raw reply
	if err := json.Unmarshal([]byte(resp.Text), &raw); err != nil {
		return Result{}, "", fmt.Errorf("intent: ответ не разобрался: %w", err)
	}
	res, repairNext := p.validate(raw, hint, text)
	return res, repairNext, nil
}

// observe записывает метрику один раз по окончательному результату: иначе
// раунд починки считался бы отказом дважды.
func (p *Parser) observe(res Result) {
	p.metrics.Observe(res.Class, res.Accepted())
}

// validate — вторая половина гарантии. Схема отвечает за форму ответа, эта
// функция за его смысл: ссылка на сущность вне сцены отклоняется, даже если
// формально валидна.
// validate возвращает результат и, если ответ можно починить одним уточнением,
// текст этого уточнения. Метрику здесь не пишем: она считается по итогу.
func (p *Parser) validate(raw reply, hint SceneHint, text string) (Result, string) {
	switch raw.Outcome {
	case OutcomeClarify:
		return Result{Clarify: fallback(raw.Clarify, "уточни, что именно ты делаешь")}, ""
	case OutcomeUnsupported:
		return Result{Candidate: fallback(raw.Reason, "словарь такого не покрывает")}, ""
	case OutcomeIntent:
	default:
		return Result{Clarify: "не разобрал, повтори иначе"}, ""
	}

	def, ok := core.LookupVerb(raw.Verb)
	if !ok {
		// Глагола нет в реестре — класс неизвестен, значит это не отказ
		// конкретного класса, а промах модели по схеме.
		return Result{Clarify: "не разобрал, повтори иначе"}, ""
	}

	reject := func(msg string) (Result, string) {
		return Result{Clarify: msg, Class: def.Class}, ""
	}

	// Прежде чем спрашивать, попробуем разрешить ссылку сами. Игрок почти
	// всегда называет персонажа по имени, а поиск имени в сцене — это
	// подстрока, а не суждение. Один сэкономленный вызов и, что важнее,
	// ход не превращается в допрос игрока о том, что он только что написал.
	need := requires(def.Verb)
	if need.Target && raw.Target == "" {
		if id, ok := resolveByName(text, hint.Entities); ok {
			raw.Target = id
		}
	}
	if need.Node && raw.Node == "" {
		if id, ok := resolveByName(text, hint.Reachable); ok {
			raw.Node = id
		}
	}

	if msg := requires(def.Verb).missing(raw); msg != "" {
		// Пропущенный обязательный аргумент — единственный случай, который
		// стоит починить: глагол угадан, не хватает ссылки на сцену.
		return Result{Clarify: msg, Class: def.Class}, "не заполнено обязательное поле — " + msg
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

	return Result{Intent: in, Class: def.Class}, ""
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
		// Слова игрока обязаны доехать до персонажа. talk_to и прочие
		// социальные глаголы своего текста не несут, и без этого актёр
		// получал пустую реплику: «поздороваться с Берном» приходило к нему
		// как молчание, и он отвечал не на приветствие, а ни на что.
		//
		// Там, где текст и есть содержание хода — theorize, say, emote, —
		// разобранное моделью важнее исходной строки: в дневник пишется
		// гипотеза, а не команда её записать.
		if res.Intent.Args.Text == "" {
			res.Intent.Args.Text = text
		}
		return res.Intent, "", nil
	case res.Candidate != "":
		// Ввод вне словаря — это не ошибка, а сигнал о его узости. Игроку
		// говорим честно, метрика уже записана.
		return nil, "так не получится: " + res.Candidate, nil
	default:
		return nil, res.Clarify, nil
	}
}
