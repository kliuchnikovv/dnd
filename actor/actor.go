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
)

func moves() []string {
	return []string{string(MoveDeflect), string(MoveAskBack),
		string(MoveConfirmKnown), string(MoveRefuse), string(MoveSmalltalk)}
}

const systemPrompt = `Ты озвучиваешь одного персонажа настольной игры.

Твоя работа — ФОРМУЛИРОВКА, а не содержание. Материал даётся ниже; сверх него
персонаж не знает ничего и сообщить ничего не может.

Сначала выбери ХОД:
- confirm_known — подтвердить РОВНО ОДИН факт из списка известного. Укажи его
  в поле fact. Реплика пересказывает только этот факт, ничего к нему не
  добавляя;
- deflect — уклониться: персонаж не знает или не хочет говорить;
- ask_back — вернуть вопрос игроку;
- refuse — отказать прямо;
- smalltalk — любезность без содержания.

Затем сформулируй реплику этим голосом. Одна-две фразы.

Категорически запрещено, и это проверяется: любые имена, места, должности,
числа, события и учреждения, которых нет в материале. Ни фермеров, ни книг
учёта, ни моргов, ни удвоенных дежурств. Если сказать нечего — это deflect,
и он совершенно нормален.

Отвечай на том же языке, на котором написан голос персонажа.`

func schemaFor(known []Known) map[string]any {
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
			"move": map[string]any{"type": "string", "enum": moves()},
			"fact": factField,
			"line": map[string]any{"type": "string", "description": "одна-две фразы прямой речи"},
		},
	}
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
	req.Role = llm.RoleActor
	req.Schema = schemaJSON(sit.Known)
	req.System = systemPrompt
	req.Input = renderPrompt(s, sit)
	if req.MaxTokens == 0 {
		req.MaxTokens = 200
	}

	resp, err := a.gw.Do(ctx, req)
	if err != nil {
		return "", err
	}
	var out struct {
		Move string `json:"move"`
		Fact string `json:"fact"`
		Line string `json:"line"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &out); err != nil {
		return "", fmt.Errorf("actor: реплика не разобралась: %w", err)
	}

	move, line := Move(out.Move), clean(out.Line)
	if !validMove(move) {
		return template(MoveDeflect, sit), nil
	}
	// Подтверждать можно только то, что дано, и только по одному.
	if move == MoveConfirmKnown && !knownHas(sit.Known, out.Fact) {
		return template(MoveDeflect, sit), nil
	}
	if move != MoveConfirmKnown && out.Fact != "" {
		return template(move, sit), nil
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
		return "А вам зачем?"
	case MoveRefuse:
		return "Нет."
	case MoveSmalltalk:
		return "Служба идёт."
	default:
		return "Не могу сказать."
	}
}

// allowedMaterial — всё, на что реплике разрешено опираться.
func allowedMaterial(s Speaker, sit Situation, factID string) []string {
	out := []string{s.Name, s.Voice}
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

func schemaJSON(known []Known) string {
	b, err := json.Marshal(schemaFor(known))
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

func renderPrompt(s Speaker, sit Situation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Персонаж: %s\nГолос: %s\nРасположение к парти: %s\n",
		s.Name, s.Voice, dispositionWord(s.Disposition))
	fmt.Fprintf(&b, "Игрок к нему: %s\n", sit.Verb)
	if sit.PlayerText != "" {
		fmt.Fprintf(&b, "Слова игрока: %s\n", sit.PlayerText)
	}
	if sit.Frame != "" {
		fmt.Fprintf(&b, "Что уже описано: %s\n", sit.Frame)
	}
	if len(sit.Known) == 0 {
		b.WriteString("Материала нет: сослаться не на что, остаётся deflect.\n")
	} else {
		b.WriteString("Материал — только это и можно подтверждать:\n")
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
	if !ok || e.Kind != store.EntityNPC || strings.TrimSpace(e.Voice) == "" {
		return Speaker{}, false
	}
	return Speaker{
		ID: string(e.ID), Name: e.Name, Voice: e.Voice,
		Disposition: g.Disposition[id],
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
		Frame:      v.Game.Flavour(res.FlavourKey),
	}, v.Req)
}
