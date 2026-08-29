// Package master даёт игре Мастера — единственную власть над миром.
//
// Две работы, и обе про мир, а не про дело:
//
//  1. Нарратор. Описывает сцену и исход прозой. Авторская затравка не
//     заменяется, а служит рамкой: Мастер её оживляет, не противореча.
//     Механические строки (бросок, «узнали», цена) остаются как есть — они
//     печатаются кодом и Мастеру не принадлежат.
//
//  2. Голос отказа. Мир не принял того, что попробовал игрок, — Мастер
//     говорит об этом языком мира. Решение он не принимает и не оспаривает:
//     текст отказа приходит из ядра, ему передаётся только он и сцена.
//
//  3. Власть над каноном. Персонаж, у которого спросили о том, чего у него
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

	"github.com/kliuchnikovv/dnd/guard"
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

// Checker — проверка прозы на два вектора газлайтинга. Мастер не вправе соврать
// про состояние так же, как персонаж: «ты уже в кузнице», «получилось», «ключ у
// тебя» — вопреки стору. Интерфейс здесь, реализация — пакет guard, тот же, что
// у актёра: «как ловится противоречие» одно на всю игру.
type Checker interface {
	Check(ctx context.Context, line string, material, state []string,
		req llm.Request) (guard.Verdict, error)
}

type Master struct {
	gw     *llm.Gateway
	schema string
	// guard — проверка прозы против дайджеста состояния. Необязательна: без неё
	// проза печатается как прежде, надеясь на промпт и read-scope Мастера.
	guard Checker
}

func New(gw *llm.Gateway) *Master {
	return &Master{gw: gw, schema: grantSchema()}
}

// WithGuard включает проверку прозы на противоречие состоянию и утечку дела.
func (m *Master) WithGuard(g Checker) *Master {
	m.guard = g
	return m
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

const briefingSystem = `Ты — Мастер настольной игры. Ты рассказываешь игроку, с чем его прислали:
кто он здесь, что случилось и чего от него ждут.

Тебе дана авторская рамка — она ГЛАВНАЯ и она про ДЕЛО. Это единственное
место в игре, где игрок обязан получить факты дела точно, поэтому:

  - НЕ ДОБАВЛЯЙ ничего по существу: ни имён, ни должностей, ни учреждений, ни
    чисел, ни времени, ни места, которых в рамке нет. Ни одной новой улики, ни
    одного предположения о виновном;
  - не убирай и не смягчай того, что в рамке есть: имя погибшего, где его
    нашли, кто прислал игрока;
  - не строй версий и не подсказывай, с чего начать. Это работа игрока.

Оживить можно только тон и погоду: два-три предложения, обращение на «вы» —
так написана вся авторская проза. Без вопросов и без обращений по имени.

Отвечай на языке рамки.`

const narrateSystem = `Ты — Мастер настольной игры. Ты описываешь игроку то, что он видит и что
только что произошло. Что именно из двух — сказано в первой строке ввода:
описание места показывает обстановку, описание исхода показывает событие. Не
путай их: на месте игрок озирается, в исходе — узнаёт, чем кончилось действие.

Тебе дана авторская рамка — она ГЛАВНАЯ. Ты её не заменяешь и ей не
противоречишь: ты её оживляешь, добавляя движение, звук, погоду, поведение
тех, кто рядом. Ничего нового по существу.

Две-три фразы, обращение к игроку на «вы» — так написана вся авторская проза,
и «ты» посреди неё читается как другой голос. Без обращений по имени и без
вопросов.
Никаких чисел, броска, служебных пометок: механику печатает не ты.

Про ДЕЛО ничего не добавляй: ни имён подозреваемых, ни улик, ни того, кто где
был. Что узнали — уже сказано в исходе, и повторять это своими словами не
надо. Дело ведёт автор.

Отвечай на языке рамки.`

const refuseSystem = `Ты — Мастер настольной игры. Мир только что НЕ ПРИНЯЛ то, что попробовал
игрок, и ты сообщаешь ему об этом.

Отказ уже решён — не тобой, и оспаривать его нельзя. Твоя работа одна:
сказать то же самое языком мира. Не «в текущей локации отсутствует цель», а
«здесь этого нет».

Одна фраза, максимум две. Обращение на «вы» — так написана вся авторская
проза. Ничего не придумывай сверху: ни новых людей, ни предметов, ни причин,
которых в отказе нет.

Чего делать НЕЛЬЗЯ:
  - подсказывать, что открыло бы путь: ни условия, ни предмета, ни имени
    того, у кого спросить. Игрок обязан догадаться сам, это правило игры;
  - обещать, что позже получится, или намекать, что попытка была близка;
  - называть числа, броски и служебные пометки: механику печатает не ты;
  - извиняться и объяснять, почему игрок неправ.

Отвечай на языке отказа.`

// Read scope Мастера — llm.Capabilities[llm.RoleNarrator].Reads: авторская
// рамка, сцена, канон мира и механический исход от ядра. Ни знания парти, ни
// правды дела в этом списке нет, и Refuse — самый узкий его случай.
//
// Refuse произносит отказ мира языком мира. Формулировку решает не Мастер:
// текст отказа приходит из ядра, и Мастер только одевает его в речь — иначе
// у «можно» появилось бы второе место правды.
//
// Мастеру передаётся ТОЛЬКО текст отказа и сцена. Требований гейта ядро
// наружу не отдаёт вовсе, поэтому подсказать, чем открыть путь, Мастеру
// структурно нечем — постановление «отказ гейта не подсказывает» держится
// границей данных, а не обещанием в промпте.
//
// Пустой отказ модель не беспокоит: вызов без нужды это деньги за шум.
func (m *Master) Refuse(ctx context.Context, refusal string, w World,
	req llm.Request) (string, error) {
	refusal = strings.TrimSpace(refusal)
	if refusal == "" {
		return "", nil
	}

	req.Role = llm.RoleNarrator
	// Одна строка не стоит дорогой модели — ровно та же причина, что у Grant.
	req.Tier = llm.TierCheap
	req.Schema = ""
	req.System = refuseSystem
	if req.MaxTokens == 0 {
		req.MaxTokens = 400
	}

	var b strings.Builder
	b.WriteString("Мир не принял это, и вот почему — своими словами скажи то же:\n  " +
		refusal + "\n")
	w.render(&b)
	req.Input = b.String()

	resp, err := m.gw.Do(ctx, req)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Text), nil
}

// Kind — что именно описывает Мастер. Выводить это из пустого исхода нельзя:
// у социального хода механики нет вовсе, и он выглядел как описание места —
// на «поздороваться» игрок получал прозу про погоду вместо разговора.
type Kind string

const (
	KindPlace   Kind = "place"   // обстановка: игрок озирается
	KindOutcome Kind = "outcome" // исход: игрок узнаёт, чем кончилось действие
	// KindBriefing — с чем игрока прислали. Ни место, ни исход: он ещё ничего
	// не видел и ничего не сделал, и путать это с обстановкой значит начинать
	// игру описанием погоды вместо дела.
	KindBriefing Kind = "briefing"
	// KindProbe — отклик на свободную пробу: игрок попробовал то, чего словарь
	// не выражает. Не исход — исхода не было: ядро хода не считало, кость не
	// трогало, канон не менялось. Свой вид, потому что инструкция здесь
	// обратная той, что у исхода: там надо показать, чем кончилось, здесь —
	// показать отклик и НИЧЕМ не кончить.
	KindProbe Kind = "probe"
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
//
// state — доверенный дайджест состояния (core.StateDigest). Пуст — проза не
// проверяется (брифинг: единственное место, где игроку легально сообщают факты
// дела, гвардить его состоянием нельзя). Непуст и гвард включён — проза
// сверяется с состоянием: соврать про владение, место, исход или знание Мастеру
// не вправе, как и персонажу.
func (m *Master) Narrate(ctx context.Context, kind Kind, frame string, w World,
	outcome []string, speaking string, state []string, req llm.Request) (string, error) {
	text, err := m.narrateOnce(ctx, kind, frame, w, outcome, speaking, "", req)
	if err != nil || text == "" {
		return text, err
	}
	if m.guard == nil || len(state) == 0 {
		return text, nil
	}
	return m.checked(ctx, kind, frame, w, outcome, speaking, state, text, req)
}

// narrateOnce — один вызов Мастера. avoid, если задан, называет утверждение,
// которого в прозе быть не должно: ремонт переспрашивает то же без него.
func (m *Master) narrateOnce(ctx context.Context, kind Kind, frame string, w World,
	outcome []string, speaking, avoid string, req llm.Request) (string, error) {
	req, ok := m.narrateRequest(kind, frame, w, outcome, speaking, avoid, req)
	if !ok {
		return "", nil
	}
	resp, err := m.gw.Do(ctx, req)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Text), nil
}

// narrateRequest собирает запрос прозы — общий путь для Do (narrateOnce) и
// Stream (NarrateStream). Второй результат ложен, если описывать нечего: пустая
// рамка у Мастера означает молчание.
func (m *Master) narrateRequest(kind Kind, frame string, w World,
	outcome []string, speaking, avoid string, req llm.Request) (llm.Request, bool) {
	if strings.TrimSpace(frame) == "" {
		return req, false
	}

	req.Role = llm.RoleNarrator
	req.Schema = ""
	req.System = narrateSystem
	if kind == KindBriefing {
		req.System = briefingSystem
	}
	if req.MaxTokens == 0 {
		// Проза плюс запас на рассуждение: обрезка здесь означает пустое
		// описание сцены, то есть игру без Мастера.
		req.MaxTokens = 900
	}

	var b strings.Builder
	switch {
	case kind == KindBriefing:
		b.WriteString("Это БРИФИНГ: расскажи, с чем игрока прислали. " +
			"Он ещё ничего не видел и ничего не делал.\n\n")
	case kind == KindPlace:
		b.WriteString("Это ОПИСАНИЕ МЕСТА: покажи, что игрок видит вокруг.\n\n")
	case kind == KindProbe:
		b.WriteString("Это СВОБОДНАЯ ПРОБА: игрок попробовал что-то, на что в игре нет " +
			"действия. Опиши отклик обстановки — что он при этом увидел, услышал, " +
			"почувствовал.\n" +
			"Ничем НЕ кончай: ничего не найдено, никто ничего не сказал, ничего не " +
			"открылось и не изменилось. О деле нового не сообщай — ни улики, ни " +
			"догадки, ни намёка на то, где искать. Мир просто ответил на прикосновение " +
			"и остался таким же.\n\n")
	default:
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
	if strings.TrimSpace(avoid) != "" {
		b.WriteString("\nВАЖНО: этого в состоянии игры нет — не утверждай, опиши то же " +
			"без этого: " + avoid + "\n")
	}
	req.Input = b.String()
	return req, true
}

// NarrateStream — потоковая проза. Неохраняемая (пустой state или без гварда)
// идёт настоящим токен-стримом через шлюз. Охраняемая генерится целиком,
// проверяется гвардом и отдаётся косметической нарезкой уже ПРОВЕРЕННОГО текста
// — утечки в дельтах не бывает по построению (инвариант «нет утечки в кадр»).
// Пустой результат заменяется авторской рамкой: молчание Мастера игрок читает
// как поломку.
func (m *Master) NarrateStream(ctx context.Context, kind Kind, frame string, w World,
	outcome []string, speaking string, state []string, req llm.Request) (llm.Stream, error) {
	if m.guard != nil && len(state) > 0 {
		text, err := m.Narrate(ctx, kind, frame, w, outcome, speaking, state, req)
		if err != nil {
			return nil, err
		}
		if text == "" {
			text = strings.TrimSpace(frame)
		}
		return llm.NewTextStream(text), nil
	}
	r, ok := m.narrateRequest(kind, frame, w, outcome, speaking, "", req)
	if !ok {
		return llm.NewTextStream(strings.TrimSpace(frame)), nil
	}
	return m.gw.Stream(ctx, r)
}

// checked проводит прозу через гвард. Противоречие состоянию (или утечка) —
// один переспрос «то же, без утверждения Х», перепроверка; при повторе или сбое
// возвращается пусто, и презентация печатает авторскую рамку. Рамка — доверенный
// текст автора: она про состояние не врёт, и нейтралью служит именно она, а не
// канцелярская заглушка.
func (m *Master) checked(ctx context.Context, kind Kind, frame string, w World,
	outcome []string, speaking string, state []string, text string,
	req llm.Request) (string, error) {
	material := proseMaterial(frame, w, outcome)
	v, err := m.guard.Check(ctx, text, material, state, req)
	if err != nil {
		// Сбой проверки — как у актёра: лучше доверенная рамка, чем непроверенная
		// проза. Рамку печатает презентация, получив пусто.
		return "", nil
	}
	if v.OK {
		return text, nil
	}
	fixed, err := m.narrateOnce(ctx, kind, frame, w, outcome, speaking, v.What, req)
	if err != nil || fixed == "" {
		return "", nil
	}
	// Отремонтированное проверяется снова: переспрос вправе подставить второе
	// противоречие вместо первого, и один круг здесь тоже один.
	if v2, err := m.guard.Check(ctx, fixed, material, state, req); err != nil || !v2.OK {
		return "", nil
	}
	return fixed, nil
}

// proseMaterial — на что прозе разрешено опираться: авторская рамка, сцена,
// сеттинг и механический исход. Правды дела и неизвестных парти фактов тут нет —
// read-scope Мастера тот же, что в промпте.
func proseMaterial(frame string, w World, outcome []string) []string {
	out := []string{frame}
	out = append(out, w.Scene...)
	if strings.TrimSpace(w.Setting) != "" {
		out = append(out, w.Setting)
	}
	out = append(out, outcome...)
	return out
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
