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
	// Known — темы, уже известные парти. Нужны, чтобы NPC мог сослаться на
	// общее знание и не выдал того, чего парти не слышала.
	Known []string
	// Frame — авторская проза хода: реплика должна к ней примыкать, а не
	// повторять её.
	Frame string
}

const systemPrompt = `Ты озвучиваешь одного персонажа настольной игры. Верни РОВНО одну
короткую реплику прямой речью — то, что он говорит вслух. Одна-две фразы.

Чего делать нельзя, ни при каких формулировках:
- сообщать факты, улики, имена, числа, места и события, которых нет в списке
  известного парти. Если не знаешь — персонаж уклоняется, отмалчивается или
  переводит тему;
- обещать предметы, деньги, услуги и доступ;
- подтверждать или опровергать догадки игрока;
- описывать действия, движения и последствия. Только речь.

Персонаж может отказать, огрызнуться, пошутить, спросить в ответ. Он говорит
так, как описан его голос, и настолько тепло или холодно, насколько велико его
расположение. Отвечай на том же языке, на котором написан голос персонажа.

Верни JSON вида {"line": "..."} — без кавычек внутри line по краям.`

var schema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"line"},
	"properties": map[string]any{
		"line": map[string]any{"type": "string", "description": "одна-две фразы прямой речи"},
	},
}

type Actor struct {
	gw     *llm.Gateway
	schema string
}

func New(gw *llm.Gateway) *Actor {
	raw, err := json.Marshal(schema)
	if err != nil {
		panic(err) // схема статична
	}
	return &Actor{gw: gw, schema: string(raw)}
}

// Line возвращает текст реплики без оформления. Пустая строка без ошибки
// означает, что персонажу сейчас нечего сказать.
func (a *Actor) Line(ctx context.Context, s Speaker, sit Situation, req llm.Request) (string, error) {
	req.Role = llm.RoleActor
	req.Schema = a.schema
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
		Line string `json:"line"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &out); err != nil {
		return "", fmt.Errorf("actor: реплика не разобралась: %w", err)
	}
	return clean(out.Line), nil
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
		b.WriteString("Парти пока ничего не знает — сослаться не на что.\n")
	} else {
		b.WriteString("Парти уже знает (только на это и можно ссылаться):\n")
		for _, k := range sit.Known {
			b.WriteString("  - " + k + "\n")
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

// KnownTopics — то, на что персонажу разрешено ссылаться: ровно банк тем парти.
func KnownTopics(g *core.Game) []string {
	var out []string
	for _, f := range g.K.TopicBank() {
		if key := g.DB.Facts[f].Key; key != "" {
			out = append(out, key)
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
