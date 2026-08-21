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
	// Known — материал: факты, на которые персонажу разрешено ссылаться.
	// Всё, чего здесь нет, персонаж сообщить не может.
	Known []Known
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
	MoveHint         Move = "hint"          // дать понять, что знает, но не скажет
)

func moves() []string {
	return []string{string(MoveDeflect), string(MoveAskBack),
		string(MoveConfirmKnown), string(MoveRefuse), string(MoveSmalltalk),
		string(MoveObserve), string(MoveVolunteer), string(MoveRaiseThread),
		string(MoveHint)}
}

const systemPrompt = `Ты озвучиваешь одного персонажа настольной игры.

Твоя работа — ФОРМУЛИРОВКА, а не содержание. Материал даётся ниже; сверх него
персонаж не знает ничего и сообщить ничего не может.

Материал двух видов, и правила у них разные:
- ФАКТЫ ДЕЛА — подтверждать можно только их, по одному и по идентификатору;
- ОБСТАНОВКА — место, погода, кто рядом, что видно. Об этом персонаж говорит
  свободно: это у всех на глазах.

Сначала выбери ХОД:
- confirm_known — подтвердить РОВНО ОДИН факт дела. Укажи его в поле fact;
- observe — сказать об обстановке: о погоде, о месте, о тех, кто рядом;
- volunteer — упомянуть то, что он знает, из списка заметок. Только если это
  ОТВЕЧАЕТ на сказанное игроком или естественно продолжает разговор. Укажи id
  в поле topic. Заметку перескажи СВОИМИ СЛОВАМИ, коротко, как в разговоре —
  не зачитывай её;
- raise_thread — напомнить о незакрытом деле с этой парти;
- hint — дать понять, что знает нечто, но говорить не станет. Что именно —
  не называть;
- ask_back — вернуть вопрос игроку, спросить о его деле;
- deflect — уклониться: персонаж не знает или не хочет говорить;
- refuse — отказать прямо;
- smalltalk — короткая любезность.

Затем сформулируй реплику этим голосом. Одна-две фразы.

Главное правило: реплика ОТВЕЧАЕТ на то, что сказал игрок. На приветствие —
приветствие, на вопрос — ответ или уклонение, на пустое — пустое. Вываливать
известное без повода нельзя: человек, который на «здравствуйте» отвечает
сводкой, звучит как автомат, а не как стражник.

Важно про уклонение: «нечего сказать» не значит «скажи ничего». Уклоняясь,
персонаж всё равно остаётся человеком — он ворчит о погоде, отшучивается,
спрашивает в ответ. Глухая стена из служебных формулировок это плохая реплика,
даже когда фактов нет.

Запрещено, и это проверяется: имена, места, должности, числа, события и
учреждения, которых нет ни в фактах, ни в обстановке. Ни фермеров, ни книг
учёта, ни моргов, ни удвоенных дежурств.

Отвечай на том же языке, на котором написан голос персонажа.`

func schemaFor(known []Known, talks []Topic, allowed []string) map[string]any {
	if len(allowed) == 0 {
		allowed = moves()
	}
	factField := map[string]any{"type": "string",
		"description": "id подтверждаемого факта; только при move=confirm_known"}
	if ids := knownIDs(known); len(ids) > 0 {
		factField["enum"] = ids
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"move", "line"},
		"properties": map[string]any{
			"move":  map[string]any{"type": "string", "enum": allowed},
			"fact":  factField,
			"topic": topicField(talks),
			"line":  map[string]any{"type": "string", "description": "одна-две фразы прямой речи"},
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
		"description": "id поднимаемой темы; только при move=volunteer"}
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

// LineGuard проверяет, не утверждает ли реплика того, чего нет в материале.
// Отдельный интерфейс, потому что проверка стоит вызова и должна быть
// отключаемой.
type LineGuard interface {
	Check(ctx context.Context, line string, material []string, req llm.Request) (bool, error)
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

// Line возвращает текст реплики без оформления. Пустая строка без ошибки
// означает, что персонажу сейчас нечего сказать.
func (a *Actor) Line(ctx context.Context, s Speaker, sit Situation, req llm.Request) (string, error) {
	act := Classify(sit.PlayerText)
	allowed := movesFor(act, sit)

	req.Role = llm.RoleActor
	req.Schema = schemaJSON(sit.Known, sit.Talks, allowed)
	req.System = systemPrompt
	req.Input = renderPrompt(s, sit, act)
	if req.MaxTokens == 0 {
		req.MaxTokens = 200
	}

	resp, err := a.gw.Do(ctx, req)
	if err != nil {
		return "", err
	}
	var out struct {
		Move  string `json:"move"`
		Fact  string `json:"fact"`
		Topic string `json:"topic"`
		Line  string `json:"line"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &out); err != nil {
		return "", fmt.Errorf("actor: реплика не разобралась: %w", err)
	}

	move, line := Move(out.Move), clean(out.Line)
	if !validMove(move) || !allowedMove(move, allowed) {
		return template(fallbackMove(allowed), sit), nil
	}
	// Подтверждать можно только то, что дано, и только по одному.
	if move == MoveConfirmKnown && !knownHas(sit.Known, out.Fact) {
		return template(MoveDeflect, sit), nil
	}
	if move != MoveConfirmKnown && out.Fact != "" {
		return template(move, sit), nil
	}
	if move == MoveVolunteer {
		topic, ok := topicByID(sit.Talks, out.Topic)
		if !ok {
			return template(MoveObserve, sit), nil
		}
		// Поднятую тему помечаем рассказанной: второй раз она прозвучит как
		// заклинивший автомат.
		if sit.MarkTold != nil {
			sit.MarkTold(topic)
		}
	}
	if line == "" || len([]rune(line)) > maxLine {
		return template(move, sit), nil
	}
	if a.guard != nil {
		ok, err := a.guard.Check(ctx, line, allowedMaterial(s, sit, out.Fact), req)
		if err != nil || !ok {
			// Реплика не прошла проверку — лучше бледно и правдиво, чем
			// живо и с выдуманным фермером.
			return template(move, sit), nil
		}
	}
	return line, nil
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
		return "Есть тут одна вещь, но это долгий разговор."
	case MoveRaiseThread:
		if len(sit.Threads) > 0 {
			return sit.Threads[0]
		}
		return "Ладно, потом."
	case MoveHint:
		return "Кое-что знаю. Но не здесь и не сейчас."
	case MoveSmalltalk, MoveObserve:
		if len(sit.Scene) > 1 {
			return "Погода — сами видите какая."
		}
		return "Да так, служба идёт."
	default:
		return "Тут я вам не помогу."
	}
}

// allowedMaterial — всё, на что реплике разрешено опираться.
func allowedMaterial(s Speaker, sit Situation, factID string) []string {
	out := []string{s.Name, s.Voice}
	// Обстановка разрешена всегда: она на виду и придумать её нельзя.
	out = append(out, sit.Scene...)
	for _, t := range sit.Talks {
		out = append(out, t.Note)
	}
	out = append(out, sit.Threads...)
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

func schemaJSON(known []Known, talks []Topic, allowed []string) string {
	b, err := json.Marshal(schemaFor(known, talks, allowed))
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
	fmt.Fprintf(&b, "Персонаж: %s\nГолос: %s\nРасположение к парти: %s\n",
		s.Name, s.Voice, dispositionWord(s.Disposition))
	fmt.Fprintf(&b, "Игрок к нему: %s (%s)\n", sit.Verb, actHint(act))
	if sit.PlayerText != "" {
		fmt.Fprintf(&b, "Слова игрока: %s\n", sit.PlayerText)
	}
	if sit.Frame != "" {
		fmt.Fprintf(&b, "Что уже описано: %s\n", sit.Frame)
	}
	if len(sit.Talks) > 0 {
		b.WriteString("Что знает и может упомянуть к месту (перескажи своими словами):\n")
		for _, t := range sit.Talks {
			b.WriteString("  " + t.ID + " — " + t.Note + "\n")
		}
	}
	if len(sit.Threads) > 0 {
		b.WriteString("Незакрытое с этой парти:\n")
		for _, t := range sit.Threads {
			b.WriteString("  " + t + "\n")
		}
	}
	if sit.KnowsSomething {
		b.WriteString("Он знает нечто, чего парти не знает; называть это нельзя.\n")
	}
	if len(sit.Scene) > 0 {
		b.WriteString("Обстановка — об этом можно говорить свободно:\n")
		for _, sc := range sit.Scene {
			b.WriteString("  " + sc + "\n")
		}
	}
	if len(sit.Known) == 0 {
		b.WriteString("Фактов дела нет: подтверждать нечего.\n")
	} else {
		b.WriteString("Факты дела — только это и можно подтверждать:\n")
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
	return v.Actor.Line(ctx, speaker, Situation{
		Verb:       string(in.Verb),
		PlayerText: in.Args.Text,
		Known:      KnownTopics(v.Game),
		Talks:      topicsOf(v.Game, in.Args.Target),
		MarkTold: func(t Topic) {
			v.Game.D.MarkTold(in.Args.Target, t.Note)
		},
		Threads:        v.Game.D.OpenThreads(in.Args.Target),
		KnowsSomething: knowsUnrevealed(v.Game, in.Args.Target),
		Scene:          SceneOf(v.Game, speaker.ID),
		Frame:          v.Game.Flavour(res.FlavourKey),
	}, v.Req)
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
