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
	"unicode"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/master"
	"github.com/kliuchnikovv/dnd/propose"
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
	// CaseNames — имена людей, вещей и мест дела. В промпт НЕ попадает
	// никогда: это список запрета для проверки озвученной заглушки, а не
	// материал. Заглушке некого называть по имени, и назвавшая — выдумка.
	CaseNames []string
	// Reveal — факт, который персонаж ГОВОРИТ прямо сейчас. Его выдал этот
	// ход, и произносит его тот, кого назвало ядро: механическая строка
	// «узнали» остаётся квитанцией, а знание игрок получает голосом.
	//
	// Не расширение материала: факт уже в Known, парти его знает. Здесь
	// сказано только, что из материала сейчас надо произнести.
	Reveal *Known
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

Как говорить. Пиши ТОЛЬКО слова, которые произносишь вслух. Без ремарок, без
описаний своих действий и без тире в начале: как ты повёл себя и что вокруг —
работа Мастера, а тире ставит сама игра. «Здравствуйте. Что вам надобно?» —
да; «— Здравствуйте. — Отряхиваю воду с плаща.» — нет.

Одна-две фразы. Отвечай на сказанное: на приветствие —
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
				"description": "одна-две фразы: только слова вслух, без ремарок и без тире"},
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
	// notify — куда сообщить, ПОЧЕМУ реплика стала заглушкой. К заглушке ведут
	// четыре разных пути, и снаружи они дают одну и ту же фразу: живой прогон
	// принял её за игнор вопроса, и разобраться удалось только чтением кода.
	// Игроку причина не показывается — это отладка, а не часть разговора.
	notify func(reason string)
}

func New(gw *llm.Gateway) *Actor { return &Actor{gw: gw} }

// WithNotify включает сообщения о том, почему реплика не получилась.
// Необязательно: без него заглушка работает как раньше, молча.
func (a *Actor) WithNotify(f func(string)) *Actor {
	a.notify = f
	return a
}

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
	if strings.TrimSpace(resp.Text) == "" {
		// Пустой ответ при полном расходе вывода — обрезка, а не молчание
		// модели. Называть это «реплика не разобралась» значит прятать
		// причину: чинится она одним числом, а не промптом.
		return Reply{}, fmt.Errorf("actor: пустой ответ, похоже упёрлись в потолок вывода (%d токенов)",
			req.MaxTokens)
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
	if out.Line == "" {
		return a.floor(ctx, s, sit, req, "модель вернула пустую строку"), nil
	}
	if len([]rune(out.Line)) > maxLine {
		// Обрезаем по границе фразы, а не выбрасываем целиком. Живой прогон:
		// ответ Нильса — по делу и в характере — превысил предел на 16 знаков
		// и был потерян весь. Нильс тараторит по авторскому замыслу («сыплет
		// подробностями»), значит предел он задевает постоянно, и выброс
		// стоил игроку речи там, где хватало точки.
		//
		// Обрезать не по чему — тогда дно: одна фраза длиннее предела это
		// поток, а не речь, и обрубок читается хуже заглушки.
		trimmed := trimToSentence(out.Line, maxLine)
		if trimmed == "" {
			return a.floor(ctx, s, sit, req, fmt.Sprintf(
				"реплика длиннее предела (%d знаков против %d) и не режется по фразе",
				len([]rune(out.Line)), maxLine)), nil
		}
		if a.notify != nil {
			a.notify(fmt.Sprintf("реплика обрезана по фразе: %d знаков против %d",
				len([]rune(out.Line)), maxLine))
		}
		out.Line = trimmed
	}
	if a.guard == nil {
		return out.Line, nil
	}
	material := allowedMaterial(s, sit, "")
	v, err := a.guard.Check(ctx, out.Line, material, req)
	if err != nil {
		// Сбой проверки трактуется как отказ: лучше бледно и правдиво, чем
		// живо и с выдуманным фермером.
		return a.floor(ctx, s, sit, req, "проверка на утечку не отработала: "+err.Error()), nil
	}
	if v.OK {
		return out.Line, nil
	}
	// Сработавшая проверка не обязана стоить голоса: сначала переспрос «то
	// же, без придуманного». Заглушка — дно после провала ремонта, а не
	// первая реакция.
	fixed, err := a.repair(ctx, s, sit, out.Line, v.What, req)
	if err != nil || fixed == "" {
		reason := "утечка не починилась (" + v.What + "): ремонт вернул пустое"
		if err != nil {
			reason = "утечка не починилась (" + v.What + "): " + err.Error()
		}
		return a.floor(ctx, s, sit, req, reason), nil
	}
	// Отремонтированное проверяется снова: ремонт вправе подставить вторую
	// выдумку вместо первой, и один круг здесь тоже один.
	if v2, err := a.guard.Check(ctx, fixed, material, req); err != nil || !v2.OK {
		reason := "утечка после ремонта (" + v.What + " → " + v2.What + ")"
		if err != nil {
			reason = "перепроверка после ремонта не отработала: " + err.Error()
		}
		return a.floor(ctx, s, sit, req, reason), nil
	}
	return fixed, nil
}

const repairPrompt = `Ты сказал фразу, в которой оказалось придумано то, чего ты знать не можешь.

Скажи ТО ЖЕ САМОЕ, тем же голосом и тем же тоном, но без придуманного.
Только слова вслух: без ремарок, без описаний своих действий, без тире.
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
	// Ремонт — правка одной фразы, но потолок всё равно с запасом на
	// рассуждение: обрезанный ремонт уронит реплику в заглушку, ради ухода
	// от которой он и затеян.
	req.MaxTokens = 400

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
//
// reason называет путь, которым сюда пришли. Он уходит в отладку, а не игроку:
// четыре пути дают одну фразу, и без причины «guard зарубил» неотличимо от
// «персонажу нечего сказать».
func (a *Actor) floor(ctx context.Context, s Speaker, sit Situation, req llm.Request,
	reason string) string {
	act := Classify(sit.PlayerText)
	tmpl := template(fallbackMove(movesFor(act, sit)), sit)

	// Смысл выбран кодом, голос просим у дешёвой модели: захардкоженная строка
	// одна на всех, и живой прогон принимал её за сбой движка — «Кто ж его
	// знает. Сыро только» звучит одинаково у стражника, мальчишки и вдовы.
	voiced, ok := a.voiceFloor(ctx, s, sit, req, tmpl)
	if a.notify != nil {
		how := "шаблоном"
		if ok {
			how = "озвучена голосом персонажа"
		}
		a.notify("реплика заменена заглушкой (" + how + "): " + reason)
	}
	if ok {
		return voiced
	}
	return tmpl
}

// voiceFloor пересказывает шаблон голосом персонажа. Второй результат — вышло
// ли: не вышло значит «отдавай шаблон дословно».
//
// Судья здесь не зовётся намеренно. Дно — уже путь сбоя, и второй вызов
// проверки на нём либо стоит ещё денег, либо уводит в круг «дно → проверка →
// дно». Вместо него три детерминированные проверки, и любая непройденная
// возвращает шаблон: выдумывать модели не из чего, потому что материала дела в
// промпте нет вовсе, а назвать кого-то по имени она не вправе.
func (a *Actor) voiceFloor(ctx context.Context, s Speaker, sit Situation,
	req llm.Request, tmpl string) (string, bool) {
	req.Role = llm.RoleActor
	req.Tier = llm.TierCheap
	req.Schema = ""
	req.System = floorPrompt
	req.MaxTokens = 200

	var b strings.Builder
	fmt.Fprintf(&b, "Ты — %s.\nТвой голос: %s\nТвоё отношение к этим людям: %s.\n",
		s.Name, s.Voice, dispositionWord(s.Disposition))
	if sit.Life != "" {
		b.WriteString("Твой день: " + sit.Life + "\n")
	}
	if sit.PlayerText != "" {
		b.WriteString("Тебе сказали: " + sit.PlayerText + "\n")
	}
	b.WriteString("\nСмысл твоего ответа — вот он, и менять его нельзя:\n  " + tmpl + "\n")
	req.Input = b.String()

	resp, err := a.gw.Do(ctx, req)
	if err != nil {
		return "", false
	}
	line := clean(resp.Text)
	if line == "" || len([]rune(line)) > maxLine {
		return "", false
	}
	// Схемы у этого вызова нет, а модель отвечает как умеет: живой тест поймал
	// пришедший сюда JSON реплики, и без проверки он уехал бы игроку дословно.
	// Заглушка — слова вслух, и фигурных скобок в них не бывает.
	if strings.ContainsAny(line, "{}") {
		return "", false
	}
	for _, name := range sit.CaseNames {
		if name != "" && name != s.Name && strings.Contains(line, name) {
			return "", false
		}
	}
	if named := namedOutsidePrompt(line, req.Input); named != "" {
		// Живой прогон: на шаблон «Кто ж его знает. Сыро только» дешёвая
		// модель ответила «у Маруси в таверне можно» — человека с таким
		// именем в деле нет вовсе. Проверка по именам дела такое не ловит
		// (выдуманного там и не будет), поэтому правило другое: назвать
		// можно только то, что в промпте уже стояло.
		return "", false
	}
	return line, true
}

// namedOutsidePrompt возвращает первое имя собственное из реплики, которого не
// было в промпте, — или пустую строку, если таких нет.
//
// Заглушке называть некого: смысл ей дали готовый, и всякое имя в ней либо
// пересказ промпта, либо выдумка. Заглавная буква — грубый признак, но в
// русском именно она отличает Марусю от таверны, а промах в эту сторону стоит
// лишь того, что игрок получит шаблон дословно.
//
// Первое слово фразы не считается: с заглавной начинается любая речь.
func namedOutsidePrompt(line, prompt string) string {
	known := strings.ToLower(prompt)
	sentenceStart := true
	for _, word := range strings.FieldsFunc(line, func(r rune) bool {
		return unicode.IsSpace(r)
	}) {
		bare := strings.TrimFunc(word, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		ends := strings.ContainsAny(word, ".!?…")
		if bare == "" {
			sentenceStart = sentenceStart || ends
			continue
		}
		first := []rune(bare)[0]
		if !sentenceStart && unicode.IsUpper(first) &&
			!strings.Contains(known, strings.ToLower(bare)) {
			return bare
		}
		sentenceStart = ends
	}
	return ""
}

const floorPrompt = `Скажи то же самое своим голосом.

Тебе дан СМЫСЛ ответа — одна строка. Твоя работа: произнести этот смысл так,
как сказал бы его именно ты, одной-двумя короткими фразами. Ничего нового не
добавляй: ни людей, ни мест, ни событий, ни чисел, ни обещаний. Никого не
называй по имени.

Пиши только слова вслух: без ремарок, без описаний своих действий и без тире в
начале. Отвечай на том же языке, на котором написан твой голос.`

// maxLine — здравый предел длины реплики. Дальше него текст обрезается по
// границе фразы (см. finish): предел стоит против потока, а не против
// подробности, и терять целую живую реплику из-за десятка лишних знаков он не
// вправе.
const maxLine = 220

// trimToSentence обрезает реплику по последней законченной фразе, влезающей в
// предел. Пусто означает, что резать не по чему.
func trimToSentence(line string, max int) string {
	runes := []rune(line)
	if len(runes) <= max {
		return strings.TrimSpace(line)
	}
	cut := -1
	for i := 0; i < max && i < len(runes); i++ {
		if strings.ContainsRune(".!?…", runes[i]) {
			cut = i
		}
	}
	if cut < 0 {
		return ""
	}
	return strings.TrimSpace(string(runes[:cut+1]))
}

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
	// Сказанное персонажем в ЭТОМ разговоре — уже его слова, а не выдумка:
	// дизайн прямо заявляет, что разговор он помнит, и промпт генерации
	// историю ему показывает. Без этой строки гвард был вправе зарубить
	// пересказ собственной реплики, и живой прогон получил канцелярскую
	// заглушку на прямой вопрос «откуда ты знаешь?».
	//
	// Строки ИГРОКА сюда не идут намеренно: иначе мир вписывается вводом —
	// назови «фермера Олсена» сам, попроси повторить, и выдуманный человек
	// станет законным материалом.
	for _, e := range sit.History {
		if e.Reply != "" {
			out = append(out, e.Reply)
		}
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
	// Тире прямой речи ставит презентация. Модель, поставившая своё, даёт
	// «— — Здравствуйте»: об оформлении обязан знать один слой, а не два.
	line = strings.TrimLeft(line, "—–-")
	return strings.TrimSpace(line)
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
	if sit.Reveal != nil {
		b.WriteString("\nСЕЙЧАС ТЫ ЭТО ГОВОРИШЬ — скажи это своими словами, " +
			"одной-двумя фразами:\n  " + sit.Reveal.Text + "\n" +
			"Это уже решено: не отказывайся, не уклоняйся и не переспрашивай. " +
			"Остальное из списка не пересказывай.\n")
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
	// OnPropose — куда сообщить вердикт ядра по предложенной мутации. Шов до
	// аудита (ADR-0002): «что предложили» расходится с «что применили» именно
	// здесь, и без этого шва отказ исчезал бы внутри озвучки — персонаж
	// промолчал, а почему, не знает никто.
	//
	// Необязателен: без него канон работает как работал, молча.
	OnPropose func(role llm.Role, m core.Mutation, app core.Applied, ref core.Refusal)
}

func (v *GameVoicer) turn() int {
	if v.Turn == nil {
		return 0
	}
	return v.Turn()
}

// Voice возвращает реплику или пустую строку, если сейчас говорить нечему.
//
// Кто произносит выданный факт, решает ядро (`TurnResult.SpokenBy`) — здесь
// это решение исполняется, а не выводится заново. Названный держатель говорит;
// если ядро не назвало никого (факт от вещи или от отсутствующего), персонаж
// молчит и факт остаётся прозой Мастера.
//
// На ходу-выдаче класс глагола не глушит: `question` — исследование, а не
// разговор, но именно им человека и спрашивают, и молчать в ответ на свой же
// выданный факт он не может. На всех прочих ходах правило прежнее: голос
// уместен там, где ход обращён к человеку.
func (v *GameVoicer) Voice(ctx context.Context, in core.Intent, res core.TurnResult) (string, error) {
	if in.Args.Target == "" {
		return "", nil
	}
	reveals := res.SpokenBy != "" && res.SpokenBy == in.Args.Target
	if len(res.Learned) > 0 && !reveals {
		return "", nil
	}
	def, ok := core.Verbs[in.Verb]
	if !ok {
		return "", nil
	}
	if !reveals && def.Class != core.ClassSocial && def.Class != core.ClassNone {
		return "", nil
	}
	speaker, ok := SpeakerFor(v.Game, in.Args.Target)
	if !ok {
		return "", nil
	}
	// Что здесь собрано — и есть read scope роли актёра
	// (llm.Capabilities[llm.RoleActor].Reads): фраза игрока, party_knowledge,
	// своё досье, гранты Мастера, сцена и авторская рамка мира. Правды дела и
	// неизвестных парти фактов тут нет и быть не может — новое поле сверять с
	// реестром, а не с соседними полями.
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
		CaseNames:      caseNames(v.Game),
	}
	if reveals {
		sit.Reveal = revealed(v.Game, res.Learned)
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
	// На ходу-выдаче круг к Мастеру не нужен: персонажу есть что сказать, и
	// добирать материал значило бы платить второй вызов на каждом факте.
	var grants []master.Grant
	var refused []string
	if sit.Reveal == nil {
		grants, refused = v.resolveNeeds(ctx, first.Needs, sit)
	}
	if len(grants) == 0 && len(refused) == 0 {
		// Мастера нет, спрашивать нечего или он не ответил — договаривать
		// нечем, и переспрашивать модель впустую значит платить за шум.
		return v.Actor.finish(ctx, speaker, sit, first, v.Req)
	}
	sit.Grants, sit.Refused, sit.MasterAnswered = grants, refused, true
	return v.Actor.Line(ctx, speaker, sit, v.Req)
}

// caseNames — имена людей, вещей и мест дела: список запрета для озвученной
// заглушки. В промпт не уходит, только в проверку.
func caseNames(g *core.Game) []string {
	var out []string
	for _, e := range g.DB.Entities {
		if e.Name != "" {
			out = append(out, e.Name)
		}
	}
	for _, l := range g.DB.Locations {
		if l.Name != "" {
			out = append(out, l.Name)
		}
	}
	return out
}

// revealed — факт этого хода в форме материала. Берётся первый: ход выдаёт
// один факт, а не список, и второй здесь означал бы, что персонаж зачитывает
// сводку.
func revealed(g *core.Game, learned []core.Learned) *Known {
	for _, l := range learned {
		if key := g.DB.Facts[l.Fact].Key; key != "" {
			return &Known{ID: string(l.Fact), Text: key}
		}
	}
	return nil
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
		if g.Canon {
			text, ok := v.canonize(g.Topic, g.Answer)
			if !ok {
				// Ядро деталь не приняло. Озвучить её всё равно значило бы
				// применить непринятое голосом персонажа: для него это тот же
				// отказ, что и молчание Мастера.
				refused = append(refused, g.Topic)
				continue
			}
			g.Answer = text
		}
		out = append(out, g)
	}
	return out, refused
}

// canonize проводит деталь Мастера через капабилити-гейт и ядро. Мастер
// говорит от роли нарратора — от неё же и предлагает.
//
// Канон держит слово: если тема уже решена, действующий ответ сильнее свежего.
// Ядро отвергает конфликт вердиктом, а не подменой, поэтому действующий ответ
// приходится взять здесь — иначе второй вопрос дал бы второй мир.
func (v *GameVoicer) canonize(topic, answer string) (string, bool) {
	m := core.Mutation{
		Kind: core.MutCanonAmbient, Target: topic, Text: answer, Delta: v.turn(),
	}
	app, ref := propose.Mutation(v.Game, llm.RoleNarrator, m)
	if v.OnPropose != nil {
		v.OnPropose(llm.RoleNarrator, m, app, ref)
	}
	if !ref.Refused() {
		// Текст берётся из вердикта, а не из предложения: применённое и
		// предложенное совпадают не всегда.
		return app.Text, true
	}
	if have, ok := v.Game.CanonGet(topic); ok {
		return have, true
	}
	return "", false
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
