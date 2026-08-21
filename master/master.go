// Package master даёт игре Мастера — единственную власть над миром.
//
// Две работы, и обе про мир, а не про дело:
//
//  1. Нарратор. Описывает сцену и исход прозой. Авторская затравка не
//     заменяется, а служит рамкой: Мастер её оживляет, не противореча.
//     Механические строки (бросок, «узнали», цена) остаются как есть — они
//     печатаются кодом и Мастеру не принадлежат.
//
//  2. Власть над каноном. Персонаж, у которого спросили о том, чего у него
//     нет, не выдумывает, а запрашивает. Отвечает Мастер: решает
//     ambient-деталь и помечает её каноном, чтобы второй вопрос вернул то же,
//     а не новую выдумку.
//
// Границы дела Мастер не переступает: имя, улика, событие вокруг преступления
// и число — территория автора дела, а не импровизации. Containment живёт
// здесь, на ОДНОЙ границе, а не размазан по каждой реплике персонажа.
//
// Пакет лежит над доменом и необязателен: без него игра работает как раньше,
// авторской прозой.
package master

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kliuchnikovv/dnd/llm"
)

// Grant — ambient-деталь, которую Мастер решил в ответ на запрос.
type Grant struct {
	// Topic — о чём деталь. Ключ канона: по нему второй вопрос узнаётся.
	Topic string `json:"topic"`
	// Answer — что в мире на этот счёт есть.
	Answer string `json:"answer"`
	// Canon — устойчивая деталь мира, которую надо запомнить. Мимолётное
	// (настроение, кто сейчас прошёл мимо) каноном не становится.
	Canon bool `json:"canon"`
}

// World — мир, в котором Мастер решает. Setting — авторская сеттинг-библия
// дела, Scene — что видно сейчас. Мир задаёт дело, а не промпт: посёлок,
// вписанный в промпт константой, был бы вторым местом правды о мире.
type World struct {
	Setting string
	Scene   []string
}

func (w World) render(b *strings.Builder) {
	if strings.TrimSpace(w.Setting) != "" {
		b.WriteString("\nМир, в котором это происходит:\n" + w.Setting + "\n")
	}
	if len(w.Scene) > 0 {
		b.WriteString("\nСцена:\n")
		for _, s := range w.Scene {
			b.WriteString("  " + s + "\n")
		}
	}
}

// CanonFact — уже решённое. Мастер обязан это видеть: иначе он решит тот же
// вопрос второй раз и по-другому.
type CanonFact struct {
	Topic string
	Text  string
}

type Master struct {
	gw     *llm.Gateway
	schema string
}

func New(gw *llm.Gateway) *Master {
	return &Master{gw: gw, schema: grantSchema()}
}

const grantSystem = `Ты — Мастер настольной игры и единственная власть над её миром.

Персонаж, которого о чём-то спросили, не нашёл ответа у себя и передал вопрос
тебе. Твоя работа — решить, что в этом мире на этот счёт есть.

Мир описан ниже: держись его уклада, погоды, ремёсел и порядков.
Отвечай коротко и конкретно, одной фразой: не «возможно, кто-то запирает», а
«ключи у смотрителя весов, он же запирает на ночь». Держись того, что уже
решено (список ниже) и того, что видно в сцене: противоречить им нельзя.

canon=true — если это устойчивая деталь мира: порядок, должность, обычай,
устройство места. Такую деталь запомнят, и второй раз она вернётся той же.
canon=false — если это мимолётное: чьё-то настроение, кто сейчас прошёл мимо.

ОТКАЖИ (впиши запрос в refuse), если вопрос — территория ДЕЛА: кто виновен,
что случилось в ночь преступления, где улика, что написано в чужих бумагах,
кто кого видел и во сколько. Дело пишет автор, не ты. Отказ — законный
исход: персонаж честно скажет, что не знает.

Не вводи имён людей, которых в сцене нет, чисел вокруг преступления и
учреждений, которых мир не предполагает.`

const narrateSystem = `Ты — Мастер настольной игры. Ты описываешь игроку то, что он видит и что
только что произошло. Что именно из двух — сказано в первой строке ввода:
описание места показывает обстановку, описание исхода показывает событие. Не
путай их: на месте игрок озирается, в исходе — узнаёт, чем кончилось действие.

Тебе дана авторская рамка — она ГЛАВНАЯ. Ты её не заменяешь и ей не
противоречишь: ты её оживляешь, добавляя движение, звук, погоду, поведение
тех, кто рядом. Ничего нового по существу.

Две-три фразы, второе лицо, без обращений к игроку по имени и без вопросов.
Никаких чисел, броска, служебных пометок: механику печатает не ты.

Про ДЕЛО ничего не добавляй: ни имён подозреваемых, ни улик, ни того, кто где
был. Что узнали — уже сказано в исходе, и повторять это своими словами не
надо. Дело ведёт автор.

Отвечай на языке рамки.`

// Kind — что именно описывает Мастер. Выводить это из пустого исхода нельзя:
// у социального хода механики нет вовсе, и он выглядел как описание места —
// на «поздороваться» игрок получал прозу про погоду вместо разговора.
type Kind string

const (
	KindPlace   Kind = "place"   // обстановка: игрок озирается
	KindOutcome Kind = "outcome" // исход: игрок узнаёт, чем кончилось действие
)

// Grant решает запросы персонажа. Пустой запрос модель не беспокоит: вызов
// без нужды это деньги за шум.
func (m *Master) Grant(ctx context.Context, needs []string, canon []CanonFact,
	w World, req llm.Request) ([]Grant, []string, error) {
	needs = nonEmpty(needs)
	if len(needs) == 0 {
		return nil, nil, nil
	}

	req.Role = llm.RoleNarrator
	// Решение ambient-детали — это «да/нет и одна строка». За него платится в
	// каждом разговоре, и дорогая модель ему не нужна.
	req.Tier = llm.TierCheap
	req.Schema = m.schema
	req.System = grantSystem
	req.MaxTokens = 900

	var b strings.Builder
	b.WriteString("Спрашивают:\n")
	for _, n := range needs {
		b.WriteString("  - " + n + "\n")
	}
	if len(canon) > 0 {
		b.WriteString("\nУже решено — этому противоречить нельзя:\n")
		for _, c := range canon {
			b.WriteString("  - " + c.Topic + ": " + c.Text + "\n")
		}
	}
	w.render(&b)
	req.Input = b.String()

	resp, err := m.gw.Do(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	var out struct {
		Grants []Grant  `json:"grants"`
		Refuse []string `json:"refuse"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &out); err != nil {
		return nil, nil, fmt.Errorf("master: ответ не разобрался: %w", err)
	}
	var grants []Grant
	for _, g := range out.Grants {
		g.Topic, g.Answer = strings.TrimSpace(g.Topic), strings.TrimSpace(g.Answer)
		if g.Topic != "" && g.Answer != "" {
			grants = append(grants, g)
		}
	}
	return grants, nonEmpty(out.Refuse), nil
}

// Narrate описывает сцену и исход прозой. Пустая рамка означает, что автор
// текста не написал: тогда описывать нечего и придумывать нечего.
func (m *Master) Narrate(ctx context.Context, kind Kind, frame string, w World,
	outcome []string, speaking string, req llm.Request) (string, error) {
	if strings.TrimSpace(frame) == "" {
		return "", nil
	}

	req.Role = llm.RoleNarrator
	req.Schema = ""
	req.System = narrateSystem
	if req.MaxTokens == 0 {
		// Проза плюс запас на рассуждение: обрезка здесь означает пустое
		// описание сцены, то есть игру без Мастера.
		req.MaxTokens = 900
	}

	var b strings.Builder
	if kind == KindPlace {
		b.WriteString("Это ОПИСАНИЕ МЕСТА: покажи, что игрок видит вокруг.\n\n")
	} else {
		b.WriteString("Это ИСХОД только что сделанного: покажи, чем оно кончилось. " +
			"Обстановку не пересказывай — игрок её уже видел.\n\n")
	}
	if strings.TrimSpace(speaking) != "" {
		b.WriteString("СЕЙЧАС ОТВЕТИТ " + speaking + " — своей репликой, следующей строкой.\n" +
			"Не говори за него и не описывай, как он себя повёл: ни его слов, ни его тона, " +
			"ни того, смягчился он или насторожился. Опиши только то, что видно вокруг и что " +
			"сделал сам игрок.\n\n")
	}
	b.WriteString("Рамка автора — главная, не противоречь ей:\n" + frame + "\n")
	w.render(&b)
	if len(outcome) > 0 {
		b.WriteString("\nЧто только что произошло:\n")
		for _, o := range outcome {
			b.WriteString("  " + o + "\n")
		}
	}
	req.Input = b.String()

	resp, err := m.gw.Do(ctx, req)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Text), nil
}

func grantSchema() string {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"grants"},
		"properties": map[string]any{
			"grants": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"topic", "answer", "canon"},
					"properties": map[string]any{
						"topic": map[string]any{"type": "string",
							"description": "о чём деталь, коротко: «ключи от весовой»"},
						"answer": map[string]any{"type": "string",
							"description": "что в мире на этот счёт есть, одной фразой"},
						"canon": map[string]any{"type": "boolean",
							"description": "устойчивая деталь мира, её надо запомнить"},
					},
				},
			},
			"refuse": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
				"description": "запросы, которые ты не вправе решать: территория дела"},
		},
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		panic(err) // схема статична, ошибка означает битый билд
	}
	return string(raw)
}

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
