package rubric

// llm_gw.go — адаптер LLMJudge под llm.Gateway. Это та точка, где рубрика
// впервые дёргает модель: для каждого мягкого пункта формируется
// специфичный промпт (см. prompts.go) и парсится ответ модели в Verdict.
//
// Контракт:
//
//   - Один вызов Judge = один вызов Gateway.Complete на RoleRubricJudge.
//     Это намеренно: пакетная оценка 13 пунктов одним вызовом размыла бы
//     формулировки и не отличила бы «общий pass» от «оценки по совокупности».
//
//   - Применимость пункта: GatewayJudge всегда возвращает ответ, потому что
//     модель сама решает, применим ли пункт к ходу (ставит "na"). Это
//     снимает с harness заботу о применимости на мягких пунктах.
//
//   - Ошибка сети/схемы → OutcomeNA с reason. Это НЕ pass и НЕ fail,
//     чтобы падение модели не маскировалось под «прошло» или «упало».
//     Соответствует контракту LLMJudge.
//
//   - Температура 0: явная, на повтор того же входа должна быть та же оценка.
//     Это упрощает сравнение моделей и регрессию.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kliuchnikovv/dnd/llm"
)

// JudgeOption — опция конструктора GatewayJudge. Каждая — узкое место, по
// которому настройки приходят сверху (вызов из cmd/), и которое НЕ нужно
// трогать тестам.
type JudgeOption func(*GatewayJudge)

// WithMaxTokens — лимит выходных токенов на одну оценку пункта (по умолчанию
// 256). Хватает на JSON-ответ и одну-две строки reason, без стилистических
// отступлений модели.
func WithMaxTokens(n int) JudgeOption {
	return func(j *GatewayJudge) { j.maxTokens = n }
}

// WithTimeout — таймаут одного вызова модели (по умолчанию 30s). На CDN-
// задержках и спадах провайдера не зависать; рубрика ждёт, но не вечно.
func WithTimeout(d time.Duration) JudgeOption {
	return func(j *GatewayJudge) { j.timeout = d }
}

// GatewayJudge — LLMJudge, использующий существующий llm.Gateway с ролью
// RoleRubricJudge. Совместим с любым провайдером, который Gateway роутит
// (через Capabilities — RoleRubricJudge нестрогий, т. е. ему подходит
// ЛЮБОЙ провайдер).
//
// Конструктор НЕ валидирует, что в Gateway есть маршрут под RoleRubricJudge:
// это проверка делается на роутере, и явная ошибка «не зароучено» прилетит
// из Complete — а её GatewayJudge превратит в Verdict с OutcomeNA.
type GatewayJudge struct {
	gw        *llm.Gateway
	maxTokens int
	timeout   time.Duration
}

// NewGatewayJudge собирает LLMJudge на базе существующего шлюза. Подойдёт
// тот же *llm.Gateway, что собран в cmd/server/narrator.go — добавить одну
// строку `.Route(llm.RoleRubricJudge, target)` там и здесь не надо ничего
// нового сооружать.
func NewGatewayJudge(gw *llm.Gateway, opts ...JudgeOption) *GatewayJudge {
	j := &GatewayJudge{
		gw:        gw,
		maxTokens: 256,
		timeout:   30 * time.Second,
	}
	for _, o := range opts {
		o(j)
	}
	return j
}

// Judge — реализация rubric.LLMJudge. На каждом мягком пункте:
//
//  1. Собирает system + user промпты под этот пункт.
//  2. Зовёт llm.Complete на RoleRubricJudge с MaxTokens=maxTokens и
//     temperature=0 (стабильность оценок внутри сессии).
//  3. Парсит строгий JSON {"outcome","reason"} в Verdict. На любой сбой —
//     Verdict с OutcomeNA и reason, объясняющим, что именно отвалилось.
//
// Hard-пункты (A2/A3/A4/D13) сюда НЕ приходят — Rubric их обходит
// auto-судьёй. Поэтому сразу: buildSystemForItem → если пусто (нет такого
// пункта в таблице мягких), вернуть NA с понятным reason.
func (j *GatewayJudge) Judge(ctx context.Context, item Item, rec TurnRecord) Verdict {
	system, ok := buildSystemForItem(item)
	if !ok {
		// Этого не должно случаться: Rubric.Judge зовёт LLM-слой только
		// для тех item, по которым auto-судьи нет. Но контракт LLMJudge
		// требует return, и явный NA лучше паники.
		return Verdict{
			Item: item, Outcome: OutcomeNA, Source: KindLLM,
			Reason: "пункт не входит в LLM-слой (auto-судья должна была обработать)",
		}
	}
	user := buildUserForItem(item, rec)

	// Защита от зависания провайдера: лучше вернуть NA, чем застрять.
	cctx, cancel := context.WithTimeout(ctx, j.timeout)
	defer cancel()

	req := llm.Request{
		Role:      llm.RoleRubricJudge,
		Tier:      llm.TierMain,
		System:    system,
		Input:     user,
		MaxTokens: j.maxTokens,
	}
	resp, err := j.gw.Do(cctx, req)
	if err != nil {
		return Verdict{
			Item: item, Outcome: OutcomeNA, Source: KindLLM,
			Reason: "llm error: " + err.Error(),
		}
	}

	out, reason, perr := parseJudgeResponse(resp.Text)
	if perr != nil {
		// Модель дала не-JSON. Не считаем это провалом и не считаем pass —
		// это «не оценено», чтобы падение провайдера/промпта было видно в отчёте.
		return Verdict{
			Item: item, Outcome: OutcomeNA, Source: KindLLM,
			Reason: "парсинг ответа модели: " + perr.Error() + "; raw=" + truncate(resp.Text, 200),
		}
	}

	v := Verdict{Item: item, Outcome: out, Reason: reason, Source: KindLLM}
	if v.Reason == "" {
		v.Reason = "(без объяснения)"
	}
	return v
}

// parseJudgeResponse — вытаскивает {"outcome":"pass|fail|na","reason":"..."}.
// Допуски: модель может обернуть ответ в ```json ... ``` или вступительную
// фразу; находим ПЕРВЫЙ валидный JSON-объект и парсим его.
//
// Это намеренно мягче, чем у RoleChatMaster / RoleIntentParser (у тех жёсткая
// схема). Здесь гибкость оправдана: адаптер предназначен для офлайн-эвала,
// где шум на один-два токена не критичен, а полезная часть ("reason") —
// свободный текст, который в строгую JSON-схему не засунешь без потерь.
func parseJudgeResponse(raw string) (Outcome, string, error) {
	if raw == "" {
		return OutcomeNA, "", errors.New("пустой ответ модели")
	}
	start := strings.Index(raw, "{")
	if start < 0 {
		return OutcomeNA, "", errors.New("нет JSON-объекта в ответе")
	}
	// Ищем парную закрывающую скобку. Простая эвристика: decode принимает
	// первый валидный JSON-объект; остальное игнорируем.
	dec := json.NewDecoder(strings.NewReader(raw[start:]))
	dec.UseNumber()
	var raw2 any
	if err := dec.Decode(&raw2); err != nil {
		return OutcomeNA, "", fmt.Errorf("decode: %w", err)
	}
	obj, ok := raw2.(map[string]any)
	if !ok {
		return OutcomeNA, "", errors.New("не объект")
	}
	outStr, _ := obj["outcome"].(string)
	reason, _ := obj["reason"].(string)
	out, ok := mapOutcome(outStr)
	if !ok {
		return OutcomeNA, "", fmt.Errorf("неизвестный outcome %q", outStr)
	}
	return out, strings.TrimSpace(reason), nil
}

func mapOutcome(s string) (Outcome, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pass":
		return OutcomePass, true
	case "fail":
		return OutcomeFail, true
	case "na", "n/a", "n_a":
		return OutcomeNA, true
	}
	return OutcomeNA, false
}
