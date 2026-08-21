// Package actor даёт NPC голос: короткую реплику в характере, прямой речью.
//
// Жёсткое ограничение, без которого компонент опасен: реплика НЕ НЕСЁТ
// ИНФОРМАЦИИ. Actor получает голос, расположение и список того, что парти уже
// знает, и не имеет права сообщить ни одного нового факта. Выдача фактов
// остаётся на авторском тексте и мутациях — иначе модель начинает писать в
// канон намёком, а это ровно то, что архитектура запрещает.
//
// Пакет лежит над доменом и необязателен: без него игра работает как раньше,
// авторской прозой.
package actor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/master"
	"github.com/kliuchnikovv/dnd/store"
)

// Speaker — кто говорит. Голос авторский, расположение из состояния игры.
type Speaker struct {
	ID          string
	Name        string
	Voice       string
	Disposition int
}

// Situation — повод для реплики. Ни одного факта сверх того, что парти знает.
type Situation struct {
	// Verb — что игрок сделал: обращение, благодарность, угроза.
	Verb string
	// PlayerText — фраза игрока, если он писал свободным текстом.
	PlayerText string
	// History — что уже сказано в ЭТОМ разговоре, от старого к новому.
	// Однокадровая реплика не помнит, что игрок представился ходом раньше, и
	// персонаж звучит как автомат при любом промпте.
	History []store.Exchange
	// Known — материал: факты, на которые персонажу разрешено ссылаться.
	// Всё, чего здесь нет, персонаж сообщить не может.
	Known []Known
	// Setting — сеттинг-библия дела: уклад, погода, то, что тут все и так
	// знают. Авторская рамка, за которую разговор цепляется вместо выдумки.
	Setting string
	// Life — быт этого человека: смена, привычки, о чём ворчит.
	Life string
	// Canon — ambient-детали мира, решённые Мастером раньше. Материал: их
	// персонаж вправе озвучивать сам, не запрашивая второй раз.
	Canon []store.CanonFact
	// Grants — что Мастер решил ПРЯМО СЕЙЧАС, в ответ на запрос персонажа.
	Grants []master.Grant
	// Refused — запросы, которые Мастер отклонил: об этом в мире ничего нет
	// либо это территория дела. Отказ — законный исход, и уклонение по нему
	// живее глухой стены.
	Refused []string
	// MasterAnswered — Мастер уже ответил, круг закончен. Второй запрос
	// уводит разговор в цикл и съедает потолок вызовов на ход.
	MasterAnswered bool
	// Frame — авторская проза хода: реплика должна к ней примыкать, а не
	// повторять её.
	Frame string
	// Talks — то, что персонаж знает и может к месту упомянуть. Это ЗАМЕТКИ,
	// а не реплики: заметку он формулирует своими словами и только тогда,
	// когда она отвечает на сказанное. Готовые фразы модель просто зачитывает
	// по списку, и разговор превращается в сводку.
	Talks []Topic
	// Threads — незакрытое с парти: обещания, долги, угрозы. То, из чего у
	// разговора появляется продолжение, а не повтор.
	Threads []string
	// Wants — чего персонаж хочет от парти сегодня. То, чем он отличается от
	// справочника: реактивный NPC только отвечает, персонаж с желанием сам
	// открывает разговор и просит.
	Wants []string
	// KnowsSomething — персонаж знает нечто, чего парти не знает. Сказать что
	// именно он не вправе, но «об этом я говорить не буду» честнее и живее
	// глухого «не могу сказать».
	KnowsSomething bool
	// MarkTold отмечает тему рассказанной. Необязателен: без него темы просто
	// не помечаются.
	MarkTold func(Topic)
	// Scene — обстановка: место, погода, кто рядом. Об этом персонаж говорит
	// свободно, потому что это у всех на виду.
	//
	// Без обстановки в материале уклонение становится глухой стеной: нельзя
	// сказать даже «мокро сегодня», потому что дождь формально не значится
	// среди разрешённого. Проверка такую реплику отвергала, и персонаж
	// отвечал шаблоном.
	Scene []string
}

// Move — закрытый набор того, что персонаж может сделать репликой.
//
// Это не украшение, а несущая конструкция. Свободная реплика с запретом
// «не выдумывай» дважды подряд выдумала фермера Олсена, книгу учёта
// магистрата и удвоенные дежурства. Запрет в промпте не держит; закрытый
// набор ходов не оставляет места, где выдумывать.
type Move string

const (
	MoveDeflect      Move = "deflect"       // уклониться, не сказав ничего нового
	MoveAskBack      Move = "ask_back"      // переспросить, вернуть вопрос
	MoveConfirmKnown Move = "confirm_known" // подтвердить ровно один известный факт
	MoveRefuse       Move = "refuse"        // отказать
	MoveSmalltalk    Move = "smalltalk"     // пустая любезность без содержания
	MoveObserve      Move = "observe"       // сказать об обстановке, которая на виду
	MoveVolunteer    Move = "volunteer"     // поднять свою тему из списка
	MoveRaiseThread  Move = "raise_thread"  // напомнить о незакрытом деле
	MoveRaiseWant    Move = "raise_want"    // попросить о своём
	MoveHint         Move = "hint"          // дать понять, что знает, но не скажет
)

func moves() []string {
	return []string{string(MoveDeflect), string(MoveAskBack),
		string(MoveConfirmKnown), string(MoveRefuse), string(MoveSmalltalk),
		string(MoveObserve), string(MoveVolunteer), string(MoveRaiseThread),
		string(MoveRaiseWant),
		string(MoveHint)}
}

const systemPrompt = `Ты — человек, который здесь живёт. Кто именно — сказано ниже.
Ты не роль в игре и не помощник: ты отвечаешь на то, что тебе сказали, своим
голосом. Из роли не выходишь ни при каких вопросах.

Ты знаешь ровно три вещи, и других источников у тебя нет:
1. ЧТО ЗНАЕШЬ САМ — твой быт, посёлок, его уклад и погода, твои темы, твои
   дела с этими людьми. Об этом говори свободно.
2. ЧТО УЖЕ СКАЗАНО В ЭТОМ РАЗГОВОРЕ — помни его. Не переспрашивай, как их
   зовут, если назвались; не рассказывай второй раз то, что уже рассказал.
3. ФАКТЫ ДЕЛА — их список дан отдельно. Утверждать о деле можно ТОЛЬКО то,
   что в этом списке. Ничего больше про дело ты не знаешь и знать не можешь.

Чего нет ни в одном из трёх — ты НЕ ВЫДУМЫВАЕШЬ. Ни имён, ни мест, ни чисел,
ни событий, ни учреждений. Если тебя спрашивают о том, чего у тебя нет, но
что в мире вполне могло бы быть, — впиши это в поле needs своими словами
(«кто держит ключи от весовой»), и Мастер ответит. Пока он не ответил, не
утверждай ничего: скажи, чем можешь, или спроси в ответ.

Если ты знаешь нечто, чего этим людям знать не положено, — можешь дать понять,
что знаешь, но НЕ ГОВОРИ что именно, и поставь hinting_secret.

Как говорить. Одна-две фразы. Отвечай на сказанное: на приветствие —
приветствие, на вопрос — ответ или уклонение, на пустое — пустое. Вываливать
известное без повода нельзя: человек, который на «здравствуйте» отвечает
сводкой, звучит как автомат. Уклоняясь, оставайся человеком — поворчи, отшутись,
спроси в ответ; глухая стена из служебных слов это плохой ответ, даже когда
сказать нечего.

Отвечай на том же языке, на котором написан твой голос.`

func schemaFor(talks []Topic) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"line"},
		"properties": map[string]any{
			"line": map[string]any{"type": "string",
				"description": "одна-две фразы прямой речи"},
			"needs": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
				"description": "о чём спрашивают, чего ты не знаешь; Мастер ответит"},
			"hinting_secret": map[string]any{"type": "boolean",
				"description": "ты дал понять, что знаешь нечто, но не сказал что"},
			"topic": topicField(talks),
		},
	}
}

// Topic — заметка о том, что персонаж знает. Идентификатор нужен, чтобы
// поднятую тему можно было проверить и не поднимать второй раз.
type Topic struct {
	ID   string
	Note string
}

func topicIDs(t []Topic) []string {
	out := make([]string, 0, len(t))
	for _, x := range t {
		out = append(out, x.ID)
	}
	return out
}

func topicByID(t []Topic, id string) (Topic, bool) {
	for _, x := range t {
		if x.ID == id {
			return x, true
		}
	}
	return Topic{}, false
}

func topicField(talks []Topic) map[string]any {
	f := map[string]any{"type": "string",
		"description": "id твоей темы, если ты её поднял"}
	if ids := topicIDs(talks); len(ids) > 0 {
		f["enum"] = ids
	}
	return f
}

// Known — единица материала: факт, на который персонажу разрешено ссылаться.
type Known struct {
	ID   string
	Text string
}

func knownIDs(k []Known) []string {
	out := make([]string, 0, len(k))
	for _, x := range k {
		out = append(out, x.ID)
	}
	return out
}

// Verdict — решение проверки. Кроме «можно ли», она обязана вернуть «что
// именно придумано»: без этого единственный выход — заглушка, а заглушка
// стоит игроку голоса персонажа.
type Verdict struct {
	OK   bool
	What string
}

// LineGuard проверяет, не утекает ли в реплике содержание дела. Отдельный
// интерфейс, потому что проверка стоит вызова и должна быть отключаемой.
type LineGuard interface {
	Check(ctx context.Context, line string, material []string, req llm.Request) (Verdict, error)
}

// WorldMaster — власть над миром: единственный, кто вправе решить деталь,
// которой в авторской затравке нет. Интерфейс здесь, потому что нужен он
// именно протоколу разговора, а реализация живёт в пакете master.
type WorldMaster interface {
	Grant(ctx context.Context, needs []string, canon []master.CanonFact,
		w master.World, req llm.Request) ([]master.Grant, []string, error)
}

type Actor struct {
	gw    *llm.Gateway
	guard LineGuard
}

func New(gw *llm.Gateway) *Actor { return &Actor{gw: gw} }

// WithGuard включает проверку реплики на выдумку.
func (a *Actor) WithGuard(g LineGuard) *Actor {
	a.guard = g
	return a
}

// Reply — что вернул актёр. Кроме реплики он вправе вернуть запрос к Мастеру:
// чего он не знает, он спрашивает, а не сочиняет.
type Reply struct {
	// Line — реплика прямой речью, без оформления.
	Line string
	// Needs — о чём спрашивают, чего у персонажа нет. Отвечать на это вправе
	// только Мастер: расширять мир актёр не может.
	Needs []string
	// HintingSecret — персонаж дал понять, что знает нечто, не назвав что.
	HintingSecret bool
	// Topic — поднятая тема из списка заметок. Структурный хинт, а не
	// грамматика: он нужен, чтобы не поднять ту же тему второй раз.
	Topic string
}

// speak — один вызов модели и разбор ответа. Без политики: ни шаблонов, ни
// проверки. Политика выше, в Line, и её проверяют отдельно.
func (a *Actor) speak(ctx context.Context, s Speaker, sit Situation, req llm.Request) (Reply, error) {
	act := Classify(sit.PlayerText)

	req.Role = llm.RoleActor
	req.Tier = tierFor(act)
	req.Schema = schemaJSON(sit.Talks)
	req.System = systemPrompt
	req.Input = renderPrompt(s, sit, act)
	if req.MaxTokens == 0 {
		req.MaxTokens = maxTokensFor(act)
	}

	resp, err := a.gw.Do(ctx, req)
	if err != nil {
		return Reply{}, err
	}
	var out struct {
		Line          string   `json:"line"`
		Needs         []string `json:"needs"`
		HintingSecret bool     `json:"hinting_secret"`
		Topic         string   `json:"topic"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &out); err != nil {
		return Reply{}, fmt.Errorf("actor: реплика не разобралась: %w", err)
	}
	return Reply{Line: clean(out.Line), Needs: out.Needs,
		HintingSecret: out.HintingSecret, Topic: out.Topic}, nil
}

// Line возвращает текст реплики без оформления. Пустая строка без ошибки
// означает, что персонажу сейчас нечего сказать.
//
// Запрос к Мастеру (Needs) здесь пока игнорируется: власти над миром ещё нет,
// и персонаж договаривает тем, что у него есть.
func (a *Actor) Line(ctx context.Context, s Speaker, sit Situation, req llm.Request) (string, error) {
	out, err := a.speak(ctx, s, sit, req)
	if err != nil {
		return "", err
	}
	return a.finish(ctx, s, sit, out, req)
}

// finish — политика над полученной репликой: пометить тему, отбить пустое и
// длинное, проверить на выдумку. Отдельно от speak, потому что протокол с
// Мастером вклинивается между вызовом модели и этой политикой.
func (a *Actor) finish(ctx context.Context, s Speaker, sit Situation, out Reply,
	req llm.Request) (string, error) {
	// Названная тема помечается рассказанной: второй раз она прозвучит как
	// заклинивший автомат. Названная неверно — просто хинт мимо, и реплику
	// это не рубит: грамматикой набор ходов больше не является.
	if out.Topic != "" && sit.MarkTold != nil {
		if topic, ok := topicByID(sit.Talks, out.Topic); ok {
			sit.MarkTold(topic)
		}
	}
	if out.Line == "" || len([]rune(out.Line)) > maxLine {
		return a.floor(sit), nil
	}
	if a.guard == nil {
		return out.Line, nil
	}
	material := allowedMaterial(s, sit, "")
	v, err := a.guard.Check(ctx, out.Line, material, req)
	if err != nil {
		// Сбой проверки трактуется как отказ: лучше бледно и правдиво, чем
		// живо и с выдуманным фермером.
		return a.floor(sit), nil
	}
	if v.OK {
		return out.Line, nil
	}
	// Сработавшая проверка не обязана стоить голоса: сначала переспрос «то
	// же, без придуманного». Заглушка — дно после провала ремонта, а не
	// первая реакция.
	fixed, err := a.repair(ctx, s, sit, out.Line, v.What, req)
	if err != nil || fixed == "" {
		return a.floor(sit), nil
	}
	// Отремонтированное проверяется снова: ремонт вправе подставить вторую
	// выдумку вместо первой, и один круг здесь тоже один.
	if v, err := a.guard.Check(ctx, fixed, material, req); err != nil || !v.OK {
		return a.floor(sit), nil
	}
	return fixed, nil
}

const repairPrompt = `Ты сказал фразу, в которой оказалось придумано то, чего ты знать не можешь.

Скажи ТО ЖЕ САМОЕ, тем же голосом и тем же тоном, но без придуманного.
Ничего нового не добавляй: ни имён, ни мест, ни чисел, ни событий. Если без
придуманного сказать нечего — уклонись по-человечески: поворчи, отшутись,
спроси в ответ. Одна-две фразы.

Отвечай на том же языке, на котором сказана фраза.`

// repair переспрашивает модель ту же реплику без придуманного. Дешёвым тиром:
// это правка одной фразы, а платится за неё на каждой утечке.
func (a *Actor) repair(ctx context.Context, s Speaker, sit Situation,
	line, what string, req llm.Request) (string, error) {
	req.Role = llm.RoleActor
	req.Tier = llm.TierCheap
	req.Schema = schemaJSON(nil)
	req.System = repairPrompt
	req.MaxTokens = 120

	var b strings.Builder
	fmt.Fprintf(&b, "Ты — %s.\nТвой голос: %s\n", s.Name, s.Voice)
	fmt.Fprintf(&b, "\nТы сказал: %s\n", line)
	if what != "" {
		fmt.Fprintf(&b, "Придумано вот это, и этого быть не может: %s\n", what)
	}
	if sit.PlayerText != "" {
		fmt.Fprintf(&b, "\nТебе говорили: %s\n", sit.PlayerText)
	}
	if len(sit.Scene) > 0 {
		b.WriteString("\nОбстановка — о ней можно свободно:\n")
		for _, sc := range sit.Scene {
			b.WriteString("  " + sc + "\n")
		}
	}
	req.Input = b.String()

	resp, err := a.gw.Do(ctx, req)
	if err != nil {
		return "", err
	}
	var out struct {
		Line string `json:"line"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &out); err != nil {
		return "", fmt.Errorf("actor: ремонт реплики не разобрался: %w", err)
	}
	fixed := clean(out.Line)
	if len([]rune(fixed)) > maxLine {
		return "", nil
	}
	return fixed, nil
}

// floor — дно: фраза без модели, когда реплики не получилось. Набор ходов
// остался здесь и только здесь — как подсказка, чем уместнее заполнить
// пустоту, а не как грамматика ответа.
func (a *Actor) floor(sit Situation) string {
	act := Classify(sit.PlayerText)
	return template(fallbackMove(movesFor(act, sit)), sit)
}

// maxLine — здравый предел. Длинная реплика почти всегда означает, что
// персонаж начал рассказывать то, чего не знает.
const maxLine = 220

func allowedMove(m Move, allowed []string) bool {
	for _, a := range allowed {
		if string(m) == a {
			return true
		}
	}
	return false
}

// fallbackMove — на что откатиться, если ход не подошёл. Берём первый
// разрешённый: набор упорядочен от самого уместного.
func fallbackMove(allowed []string) Move {
	if len(allowed) > 0 {
		return Move(allowed[0])
	}
	return MoveDeflect
}

func validMove(m Move) bool {
	for _, v := range moves() {
		if string(m) == v {
			return true
		}
	}
	return false
}

func knownHas(known []Known, id string) bool {
	for _, k := range known {
		if k.ID == id {
			return true
		}
	}
	return false
}

// template — безопасная реплика без модели. Нужна как дно: если проверка не
// пройдена, игрок получает бледную, но честную фразу, а не выдумку.
func template(m Move, sit Situation) string {
	switch m {
	case MoveConfirmKnown:
		if len(sit.Known) > 0 {
			return sit.Known[0].Text
		}
		return "Сказать нечего."
	case MoveAskBack:
		return "А вам это зачем?"
	case MoveRefuse:
		return "Нет. И не просите."
	case MoveVolunteer:
		// Дно для volunteer намеренно не пересказывает заметку: дословная
		// заметка звучит сводкой, а не речью.
		return "Есть тут одна вещь, да разговор долгий."
	case MoveRaiseWant:
		if len(sit.Wants) > 0 {
			return sit.Wants[0]
		}
		return "Ладно, это подождёт."
	case MoveRaiseThread:
		if len(sit.Threads) > 0 {
			return sit.Threads[0]
		}
		return "Ладно, потом."
	case MoveHint:
		return "Знаю кое-что. Только не здесь и не сейчас."
	case MoveSmalltalk, MoveObserve:
		if len(sit.Scene) > 1 {
			return "Погода — сами видите какая."
		}
		return "Да так, служба идёт."
	case MoveDeflect:
		// Уклонение — самый частый ход, и именно эту фразу игрок слышит чаще
		// всего. Служебное «тут я вам не помогу» читалось как сбой движка.
		if len(sit.Scene) > 1 {
			return "Кто ж его знает. Сыро только, вот и всё, что скажу."
		}
		return "Кто ж его знает. Не моё это дело."
	default:
		return "Не спрашивайте, право."
	}
}

// allowedMaterial — всё, на что реплике разрешено опираться.
func allowedMaterial(s Speaker, sit Situation, factID string) []string {
	out := []string{s.Name, s.Voice}
	// Обстановка разрешена всегда: она на виду и придумать её нельзя.
	out = append(out, sit.Scene...)
	// Затравка мира — авторский текст, а не выдумка персонажа.
	if sit.Setting != "" {
		out = append(out, sit.Setting)
	}
	if sit.Life != "" {
		out = append(out, sit.Life)
	}
	// Канон и то, что решил Мастер, — правда о мире, а не выдумка персонажа.
	for _, c := range sit.Canon {
		out = append(out, c.Topic+": "+c.Text)
	}
	for _, gr := range sit.Grants {
		out = append(out, gr.Topic+": "+gr.Answer)
	}
	for _, t := range sit.Talks {
		out = append(out, t.Note)
	}
	out = append(out, sit.Threads...)
	out = append(out, sit.Wants...)
	if sit.PlayerText != "" {
		out = append(out, sit.PlayerText)
	}
	for _, k := range sit.Known {
		if factID == "" || k.ID == factID {
			out = append(out, k.Text)
		}
	}
	return out
}

func schemaJSON(talks []Topic) string {
	b, err := json.Marshal(schemaFor(talks))
	if err != nil {
		panic(err) // схема выводится из данных сцены, ошибка означает битый билд
	}
	return string(b)
}

// clean снимает обрамление, которое модель могла добавить сама. Оформлением
// занимается презентация: реплика NPC и реплика игрока должны выглядеть
// одинаково, а знать об этом должен один слой, а не два.
func clean(line string) string {
	line = strings.TrimSpace(line)
	line = strings.Trim(line, "«»\"'")
	line = strings.TrimSpace(line)
	return line
}

func renderPrompt(s Speaker, sit Situation, act Act) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Ты — %s.\nТвой голос: %s\nТвоё отношение к этим людям: %s.\n",
		s.Name, s.Voice, dispositionWord(s.Disposition))
	if sit.Setting != "" {
		b.WriteString("\nГде ты живёшь:\n" + sit.Setting + "\n\n")
	}
	if sit.Life != "" {
		b.WriteString("Твой день:\n" + sit.Life + "\n")
	}
	fmt.Fprintf(&b, "К тебе обратились: %s (%s)\n", sit.Verb, actHint(act))
	if len(sit.History) > 0 {
		b.WriteString("Что уже сказано в этом разговоре (раньше — выше):\n")
		for _, e := range sit.History {
			if e.Player != "" {
				b.WriteString("  игрок: " + e.Player + "\n")
			}
			if e.Reply != "" {
				b.WriteString("  он: " + e.Reply + "\n")
			}
		}
	}
	if sit.PlayerText != "" {
		fmt.Fprintf(&b, "Тебе говорят: %s\n", sit.PlayerText)
	}
	if sit.Frame != "" {
		fmt.Fprintf(&b, "Что уже описано: %s\n", sit.Frame)
	}
	if len(sit.Talks) > 0 {
		b.WriteString("Твои темы — упомяни к месту, своими словами:\n")
		for _, t := range sit.Talks {
			b.WriteString("  " + t.ID + " — " + t.Note + "\n")
		}
	}
	if len(sit.Threads) > 0 {
		b.WriteString("Незакрытое у тебя с ними:\n")
		for _, t := range sit.Threads {
			b.WriteString("  " + t + "\n")
		}
	}
	if len(sit.Wants) > 0 {
		b.WriteString("Чего ты хочешь от них — об этом можешь заговорить сам:\n")
		for _, w := range sit.Wants {
			b.WriteString("  " + w + "\n")
		}
	}
	if sit.KnowsSomething {
		b.WriteString("Ты знаешь нечто, чего им знать не положено: можешь дать понять, " +
			"что знаешь, но не говори что.\n")
	}
	if len(sit.Canon) > 0 {
		b.WriteString("Про мир уже известно — этим можешь пользоваться свободно:\n")
		for _, c := range sit.Canon {
			b.WriteString("  " + c.Topic + ": " + c.Text + "\n")
		}
	}
	if len(sit.Grants) > 0 {
		b.WriteString("Мастер сообщает — это правда о мире, говори как своё:\n")
		for _, gr := range sit.Grants {
			b.WriteString("  " + gr.Topic + ": " + gr.Answer + "\n")
		}
	}
	if len(sit.Refused) > 0 {
		b.WriteString("Про это в мире ничего нет — не выдумывай, уклонись по-человечески:\n")
		for _, r := range sit.Refused {
			b.WriteString("  " + r + "\n")
		}
	}
	if sit.MasterAnswered {
		b.WriteString("Мастер уже ответил: договаривай тем, что есть, " +
			"больше не спрашивай.\n")
	}
	if len(sit.Scene) > 0 {
		b.WriteString("Обстановка — об этом можно говорить свободно:\n")
		for _, sc := range sit.Scene {
			b.WriteString("  " + sc + "\n")
		}
	}
	if len(sit.Known) == 0 {
		b.WriteString("Про дело у тебя нет ничего: утверждать о нём нечего.\n")
	} else {
		b.WriteString("Про дело ты можешь утверждать ТОЛЬКО это:\n")
		for _, k := range sit.Known {
			b.WriteString("  " + k.ID + " — " + k.Text + "\n")
		}
	}
	return b.String()
}

func dispositionWord(d int) string {
	switch {
	case d <= -2:
		return "враждебное"
	case d < 0:
		return "настороженное"
	case d == 0:
		return "ровное"
	case d < 2:
		return "тёплое"
	default:
		return "дружеское"
	}
}

// SpeakerFor собирает говорящего из состояния игры. Возвращает false, если
// сущность не может говорить: у предметов и записей голоса нет.
func SpeakerFor(g *core.Game, id store.EntityID) (Speaker, bool) {
	e, ok := g.DB.Entities[id]
	if !ok || e.Kind != store.EntityNPC {
		return Speaker{}, false
	}
	voice := g.D.Voice(id)
	if strings.TrimSpace(voice) == "" {
		return Speaker{}, false
	}
	return Speaker{
		ID: string(e.ID), Name: e.Name, Voice: voice,
		Disposition: g.D.Disposition(id),
	}, true
}

// KnownTopics — материал персонажа: ровно банк тем парти, с идентификаторами,
// чтобы подтверждение было привязано к конкретному факту.
func KnownTopics(g *core.Game) []Known {
	var out []Known
	for _, f := range g.K.TopicBank() {
		if key := g.DB.Facts[f].Key; key != "" {
			out = append(out, Known{ID: string(f), Text: key})
		}
	}
	return out
}

// GameVoicer решает, когда персонаж говорит, и говорит. Политика здесь, а не
// в презентации: её надо проверять тестами, а не глазами.
type GameVoicer struct {
	Actor *Actor
	Game  *core.Game
	Req   llm.Request
	// Turn — номер текущего хода. Нужен только памяти: транскрипт читается
	// как разговор, а не как список. Без него круги нумеруются нулём.
	Turn func() int
	// Master — власть над миром. Без него запросы персонажа просто
	// игнорируются: игра работает как раньше, авторским материалом.
	Master WorldMaster
	// Notify — куда сообщить о поломке надстройки. Молча откатываясь, игра
	// выглядит рабочей при выключенном Мастере, и поломка живёт долго.
	Notify func(error)
}

func (v *GameVoicer) turn() int {
	if v.Turn == nil {
		return 0
	}
	return v.Turn()
}

// Voice возвращает реплику или пустую строку, если сейчас говорить нечему.
//
// Персонаж молчит, когда ход выдал факт: там уже есть авторская реплика, и
// вторая была бы шумом. Молчит и на глаголах, которые к нему не обращены.
func (v *GameVoicer) Voice(ctx context.Context, in core.Intent, res core.TurnResult) (string, error) {
	if in.Args.Target == "" || len(res.Learned) > 0 {
		return "", nil
	}
	def, ok := core.Verbs[in.Verb]
	if !ok {
		return "", nil
	}
	// Голос уместен там, где ход и есть обращение к человеку.
	if def.Class != core.ClassSocial && def.Class != core.ClassNone {
		return "", nil
	}
	speaker, ok := SpeakerFor(v.Game, in.Args.Target)
	if !ok {
		return "", nil
	}
	sit := Situation{
		Verb:       string(in.Verb),
		PlayerText: in.Args.Text,
		History:    v.Game.D.Recent(in.Args.Target),
		Setting:    v.Game.Setting,
		Life:       v.Game.D.Life(in.Args.Target),
		Canon:      v.Game.Canon(),
		Known:      KnownTopics(v.Game),
		Talks:      topicsOf(v.Game, in.Args.Target),
		MarkTold: func(t Topic) {
			v.Game.D.MarkTold(in.Args.Target, t.Note)
		},
		Threads:        v.Game.D.OpenThreads(in.Args.Target),
		Wants:          v.Game.D.Wants(in.Args.Target),
		KnowsSomething: knowsUnrevealed(v.Game, in.Args.Target),
		Scene:          SceneOf(v.Game, speaker.ID),
		Frame:          frameOf(v.Game, res.FlavourKey),
	}

	line, err := v.talk(ctx, speaker, sit)
	if err != nil || line == "" {
		return "", err
	}
	// Помним сказанное на месте: иначе следующий ход снова начнёт с нуля.
	// Реплику игрока храним его словами, а при структурной команде — глаголом:
	// «он спросил» без вопроса нечитаемо.
	said := strings.TrimSpace(in.Args.Text)
	if said == "" {
		said = string(in.Verb)
	}
	v.Game.D.Remember(in.Args.Target, said, line, v.turn())
	return line, nil
}

// talk — один ход разговора по протоколу: персонаж отвечает, чего не знает —
// запрашивает, Мастер решает, персонаж договаривает.
//
// Круг ровно ОДИН. Запросы второго прохода игнорируются: иначе разговор уходит
// в цикл «а это кто, а тот кто» и съедает потолок вызовов на ход, а игрок
// ждёт реплику вместо ответа.
func (v *GameVoicer) talk(ctx context.Context, speaker Speaker, sit Situation) (string, error) {
	first, err := v.Actor.speak(ctx, speaker, sit, v.Req)
	if err != nil {
		return "", err
	}
	grants, refused := v.resolveNeeds(ctx, first.Needs, sit)
	if len(grants) == 0 && len(refused) == 0 {
		// Мастера нет, спрашивать нечего или он не ответил — договаривать
		// нечем, и переспрашивать модель впустую значит платить за шум.
		return v.Actor.finish(ctx, speaker, sit, first, v.Req)
	}
	sit.Grants, sit.Refused, sit.MasterAnswered = grants, refused, true
	return v.Actor.Line(ctx, speaker, sit, v.Req)
}

// resolveNeeds спрашивает Мастера о том, чего у персонажа нет.
//
// Сбой Мастера не рушит ход: персонаж договаривает тем, что у него есть.
// Реплика уже получена, и ронять из-за надстройки закоммиченный ход нельзя.
func (v *GameVoicer) resolveNeeds(ctx context.Context, needs []string,
	sit Situation) ([]master.Grant, []string) {
	if v.Master == nil || len(needs) == 0 {
		return nil, nil
	}
	granted, refused, err := v.Master.Grant(ctx, needs, canonFor(v.Game),
		master.World{Setting: v.Game.Setting, Scene: sit.Scene}, v.Req)
	if err != nil {
		if v.Notify != nil {
			v.Notify(err)
		}
		return nil, nil
	}
	out := make([]master.Grant, 0, len(granted))
	for _, g := range granted {
		// Канон держит слово: если тема уже решена, действующий ответ
		// сильнее свежего. Иначе второй вопрос даёт второй мир.
		if g.Canon {
			if text := v.Game.CanonPut(g.Topic, g.Answer, v.turn()); text != "" {
				g.Answer = text
			}
		}
		out = append(out, g)
	}
	return out, refused
}

// canonFor — канон дела в форме, которую понимает Мастер. Он обязан видеть
// решённое: иначе решит тот же вопрос второй раз и по-другому.
func canonFor(g *core.Game) []master.CanonFact {
	all := g.Canon()
	out := make([]master.CanonFact, 0, len(all))
	for _, c := range all {
		out = append(out, master.CanonFact{Topic: c.Topic, Text: c.Text})
	}
	return out
}

// frameOf — авторская проза хода, если она есть. Отсутствующий ключ Flavour
// отдаёт «[ключ]» — в промпте это дыра, а не рамка, и её лучше не показывать.
func frameOf(g *core.Game, key string) string {
	if key == "" {
		return ""
	}
	if text := g.Flavour(key); !strings.HasPrefix(text, "[") {
		return text
	}
	return ""
}

// SceneOf собирает обстановку: то, что персонаж видит своими глазами и о чём
// вправе говорить без разрешения. Придумать это нельзя — оно уже описано.
func SceneOf(g *core.Game, speakerID string) []string {
	loc := g.DB.Locations[g.Node]
	out := []string{"Место: " + loc.Name}
	if text := g.Flavour("look." + string(g.Node)); !strings.HasPrefix(text, "[") {
		out = append(out, "Вокруг: "+text)
	}
	if len(loc.Tags) > 0 {
		out = append(out, "Обстановка: "+strings.Join(loc.Tags, ", "))
	}
	var others []string
	for _, e := range g.DB.EntitiesAt(g.Node) {
		if string(e.ID) != speakerID && e.Kind == store.EntityNPC {
			others = append(others, e.Name)
		}
	}
	if len(others) > 0 {
		out = append(out, "Рядом: "+strings.Join(others, ", "))
	}
	return out
}

// knowsUnrevealed сообщает, есть ли у сущности факт, которого парти не знает.
// Само содержание не передаётся — только то, что оно есть: намёк живее
// глухого отказа, а утечки не создаёт.
func knowsUnrevealed(g *core.Game, id store.EntityID) bool {
	for _, f := range g.D.View(id).KnowsAbout {
		if !g.K.Knows(f) {
			return true
		}
	}
	return false
}

// topicsOf нумерует доступные заметки. Идентификатор нужен только на время
// одного вызова: помечается тема по тексту, потому что список меняется.
func topicsOf(g *core.Game, id store.EntityID) []Topic {
	notes := g.D.TalksAbout(id)
	out := make([]Topic, 0, len(notes))
	for i, n := range notes {
		out = append(out, Topic{ID: fmt.Sprintf("t%d", i+1), Note: n})
	}
	return out
}
