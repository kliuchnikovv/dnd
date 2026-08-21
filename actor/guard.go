package actor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kliuchnikovv/dnd/llm"
)

// Guard — проверка реплики на утечку дела. Задача проще генерации: сверить
// одну короткую фразу с коротким списком материала, и потому её можно отдать
// дешёвой модели.
//
// Порог сдвинут сознательно. Прежняя формулировка «выдумка — любой новый факт
// о мире, даже мелкий» отвергала «спину сорвал» так же, как выдуманного
// фермера Олсена, и отвергнутая реплика падала в канцелярскую заглушку. Цена
// ошибок не симметрична: пропущенная бытовая подробность не стоит ничего,
// зарубленная — стоит игроку живой речи. Поэтому в сомнении реплика проходит,
// а ловится ровно утечка ДЕЛА.
type Guard struct {
	gw     *llm.Gateway
	schema string
}

const guardPrompt = `Ты проверяешь одну реплику персонажа детективной игры на УТЕЧКУ ДЕЛА.

Ниже материал — всё, что персонаж вправе знать: его быт и посёлок, обстановка,
уже решённое о мире, факты дела, известные игрокам.

Утечка — это конкретика ПРО ДЕЛО вне материала: имя человека, причастного к
преступлению, улика, чужая бумага и что в ней, место или время, где что-то
случилось, число вокруг преступления, учреждение, которого мир не
предполагает. Такую реплику надо остановить: её автор — автор дела, а не
персонаж.

НЕ утечка, и это важнее всего остального:
- бытовая краска: погода, сырость, спина, остывший чай, промокшие сапоги,
  усталость, цены, работа, ворчание на начальство;
- эмоция, грубость, шутка, вежливость, обращение к собеседнику;
- уклонение, отказ, встречный вопрос, «не знаю», «не моё дело»;
- пересказ материала другими словами, в том числе неточный;
- разговор о себе и своём дне, даже с подробностями, которых в материале
  дословно нет, — если они бытовые и к делу не относятся.

Сомневаешься между «краска» и «утечка» — это краска. Ложное срабатывание
стоит игроку живой речи, поэтому пропустить бытовую подробность дешевле, чем
зарубить её.

Верни {"invented": true|false, "what": "что именно про дело придумано"}.
В what назови только придуманное — коротко, чтобы персонаж мог сказать то же
без него.`

func NewGuard(gw *llm.Gateway) *Guard {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"invented"},
		"properties": map[string]any{
			"invented": map[string]any{"type": "boolean"},
			"what":     map[string]any{"type": "string"},
		},
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	return &Guard{gw: gw, schema: string(raw)}
}

// Check судит реплику. Возвращает вердикт: можно ли показывать, и если нет —
// что именно придумано. «Что» нужно ремонту: без него остаётся только
// заглушка, а она стоит игроку голоса персонажа.
func (g *Guard) Check(ctx context.Context, line string, material []string, req llm.Request) (Verdict, error) {
	req.Role = llm.RoleCanonGuard
	req.Tier = g.tier()
	req.Schema = g.schema
	req.System = guardPrompt
	req.MaxTokens = 200

	var b strings.Builder
	b.WriteString("Материал:\n")
	for _, m := range material {
		if strings.TrimSpace(m) != "" {
			b.WriteString("  - " + m + "\n")
		}
	}
	b.WriteString("\nРеплика: " + line + "\n")
	req.Input = b.String()

	resp, err := g.gw.Do(ctx, req)
	if err != nil {
		return Verdict{}, err
	}
	var out struct {
		Invented bool   `json:"invented"`
		What     string `json:"what"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &out); err != nil {
		return Verdict{}, fmt.Errorf("actor: проверка не разобралась: %w", err)
	}
	return Verdict{OK: !out.Invented, What: strings.TrimSpace(out.What)}, nil
}

// tier — проверка на выдумку это «да/нет». Она ничего не сочиняет, и дорогая
// модель ей не нужна никогда.
func (g *Guard) tier() llm.Tier { return llm.TierCheap }
