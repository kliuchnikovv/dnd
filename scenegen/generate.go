package scenegen

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kliuchnikovv/dnd/llm"
)

// Генератор сцены — ПРЕ-ПАСС (не роль в петле). Тема → LLM → SceneSpec → Repair →
// Validate (ретрай ≤3 с ошибками валидатора обратно в промпт) → пригодный спек.
// Правду генератор рождает как АВТОР — она уходит ядру/стражу, не Мастеру
// (анти-лик конструкцией, ADR-0008). LLM даёт битые сцены, поэтому валидатор
// обязателен. Живой прогон — с ключом; офлайн — фолбэк-ошибка/фейк-шлюз в тестах.

// GenAttempt — одна попытка генерации: сырой ответ + ошибки валидатора.
type GenAttempt struct {
	Raw    string   `json:"raw"`
	Errors []string `json:"errors,omitempty"`
}

// GenTrace — след генерации для записи/оценки (тема/режим/попытки/принятый спек).
type GenTrace struct {
	Theme    string       `json:"theme"`
	Mode     string       `json:"mode"`
	OK       bool         `json:"ok"`
	Attempts []GenAttempt `json:"attempts"`
	Spec     *SceneSpec   `json:"spec,omitempty"`
}

const genSystem = `Ты — АВТОР коротких хоррор-ван-шотов для текстовой игры в духе народных быличек. Придумай ОДНУ сцену-загадку и выдай СТРОГО как JSON по схеме — это данные для движка, не рассказ игроку.

ЖАНР: короткая жуткая сцена с ОБМАНОМ. Что-то манит или притворяется (свет, голос, радушный хозяин, гость у двери), но правда иная. Есть приманка/обман, угроза и спасение — видное внимательному. Тон сдержанный, деревенский.

РЕЖИМЫ (mode):
- "traverse": пройти опасное место (goal шагов), не поверив приманке; сойти = гибель. НУЖЕН hazard.
- "disable": обезвредить механизм/существо (win_target — id объекта) снаряжением; рядом опасная зона. НУЖЕН hazard и win_target.
- "hold": достоять goal ходов, не совершив рокового шага (впустить/открыть). НУЖНЫ beats (эскалация по ходам) и lose_text.

ОБЪЕКТЫ: 2–4. У каждого id (латиницей), name, surface (что видно СРАЗУ, без тайны), aspect (look|listen|watch), tiers (shallow→deep). Ключевой намёк-tell ставь passive=11 (осмотр открывает гарантированно); глубже — passive=0 (по броску). У каждого видимого объекта хотя бы один passive-tell (порог 10–12). grants — короткий ключ-факт или "".

TRUTH: ответ-ключ — что на самом деле, в чём обман, как победить/проиграть. Уходит движку, не игроку.

Верни СТРОГО JSON без markdown:
{"title","intro","truth","mode","ambient","safe_note","win_text","lose_text","goal","win_target","hazard":{"enter_text","depths":[...],"lose_text"}|null,"beats":[...],"objects":[{"id","name","surface","hidden":false,"aspect","tiers":[{"text","passive","grants"}]}]}

traverse/disable: заполни hazard (enter_text, 3 depths по нарастанию, lose_text), beats=[]. hold: hazard=null, beats (goal-1 штук) и lose_text.`

// Generate рождает валидную сцену через шлюз. Возвращает спек, след и ошибку.
// gw==nil или сбой всех попыток → ошибка (сцену не выдумываем из воздуха).
func Generate(ctx context.Context, gw *llm.Gateway, theme, mode string) (*SceneSpec, *GenTrace, error) {
	tr := &GenTrace{Theme: theme, Mode: strings.ToLower(strings.TrimSpace(mode))}
	if gw == nil {
		return nil, tr, fmt.Errorf("генератор: нет шлюза (нужен ключ)")
	}
	user := "Придумай новую сцену."
	if theme != "" {
		user += " Тема/зерно: " + theme + "."
	}
	if m := tr.Mode; m == "traverse" || m == "disable" || m == "hold" {
		user += " Режим: " + m + "."
	} else {
		user += " Режим выбери сам."
	}

	var lastErr string
	for attempt := 0; attempt < 3; attempt++ {
		msg := user
		if lastErr != "" {
			msg = user + "\n\nПрошлый ответ был невалиден, ИСПРАВЬ: " + lastErr
		}
		resp, err := gw.Do(ctx, llm.Request{Role: llm.RoleWorldsmith, System: genSystem, Input: msg, MaxTokens: 2000})
		if err != nil {
			lastErr = err.Error()
			tr.Attempts = append(tr.Attempts, GenAttempt{Errors: []string{lastErr}})
			continue
		}
		var spec SceneSpec
		if json.Unmarshal([]byte(extractJSONBlock(resp.Text)), &spec) != nil {
			lastErr = "ответ не разобрался как JSON"
			tr.Attempts = append(tr.Attempts, GenAttempt{Raw: resp.Text, Errors: []string{lastErr}})
			continue
		}
		Repair(&spec)
		if errs := Validate(&spec); len(errs) > 0 {
			lastErr = strings.Join(errs, "; ")
			tr.Attempts = append(tr.Attempts, GenAttempt{Raw: resp.Text, Errors: errs})
			continue
		}
		tr.Attempts = append(tr.Attempts, GenAttempt{Raw: resp.Text})
		tr.OK, tr.Spec = true, &spec
		return &spec, tr, nil
	}
	return nil, tr, fmt.Errorf("генератор не выдал валидную сцену за 3 попытки: %s", lastErr)
}

// extractJSONBlock вытаскивает первый {...} блок (обёртка текстом/markdown).
func extractJSONBlock(s string) string {
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return s
}
