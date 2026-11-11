package main

// report.go — отчёт по рубрике. Чистые функции: на вход строки результатов,
// на выходе Markdown и JSONL. Ни сети, ни файлов — чтобы отчёт можно было
// тестировать и переиспользовать из CI.

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kliuchnikovv/dnd/eval/rubric"
)

// TurnResult — результат одного хода: проза Мастера + вердикты рубрики.
type TurnResult struct {
	Index    int              `json:"index"`  // 1-based номер хода
	Input    string           `json:"input"`  // заявка игрока ("" на старте)
	Note     string           `json:"note"`   // человеческая пометка из Эталона
	Prose    string           `json:"prose"`  // проза Мастера (сырая или после стража)
	Guard    string           `json:"guard"`  // вердикт стража ("" если не проверяли)
	Etalon   []rubric.Item    `json:"etalon"` // пункты, которые сработали у человека-Мастера
	Verdicts []rubric.Verdict `json:"verdicts"`
}

// EtalonGap — пункт, который сработал у человека-Мастера, но не вытянут
// ИИ-Мастером. Это главный сигнал отчёта: «нижняя планка не взята».
type EtalonGap struct {
	Turn  int         `json:"turn"`
	Item  rubric.Item `json:"item"`
	State string      `json:"state"` // fail | na | «нет вердикта»
}

// EtalonGaps — пункты из EtalonTags, которых ИИ-Мастер не вытянул.
//
// Сравнение честное в обе стороны: теги Эталона — это «что сработало у
// человека-Мастера», а не «что обязательно». Gap = «человек вытянул, ИИ нет».
// Если ИИ вытянул больше — это не gap, это хорошо.
func EtalonGaps(r *Report) []EtalonGap {
	var out []EtalonGap
	for _, turn := range r.Turns {
		if len(turn.Etalon) == 0 {
			continue
		}
		byItem := map[rubric.Item]rubric.Verdict{}
		for _, v := range turn.Verdicts {
			byItem[v.Item] = v
		}
		for _, item := range turn.Etalon {
			v, ok := byItem[item]
			if !ok {
				out = append(out, EtalonGap{Turn: turn.Index, Item: item, State: "нет вердикта"})
				continue
			}
			if v.Outcome == rubric.OutcomePass {
				continue
			}
			state := "na"
			if v.Outcome == rubric.OutcomeFail {
				state = "fail"
			}
			out = append(out, EtalonGap{Turn: turn.Index, Item: item, State: state})
		}
	}
	return out
}

// Report — весь прогон.
type Report struct {
	Model     string       `json:"model"`
	Provider  string       `json:"provider"`
	Scenario  string       `json:"scenario"`
	StartedAt time.Time    `json:"started_at"`
	Turns     []TurnResult `json:"turns"`
}

// Summary — сводка по сценарию: по каждому пункту сколько pass/fail/na.
type ItemSummary struct {
	Item  rubric.Item `json:"item"`
	Pass  int         `json:"pass"`
	Fail  int         `json:"fail"`
	NA    int         `json:"na"`
	Total int         `json:"total"`
}

// SummarizeReport — сводка по сценарию. Пункты идут в каноничном порядке
// Эталона (Items()), даже если ни разу не применились (тогда pass=0/fail=0/na=0).
func SummarizeReport(r *Report) []ItemSummary {
	counts := map[rubric.Item]*ItemSummary{}
	for _, it := range rubric.Items() {
		counts[it] = &ItemSummary{Item: it}
	}
	for _, turn := range r.Turns {
		for _, v := range turn.Verdicts {
			c, ok := counts[v.Item]
			if !ok {
				continue
			}
			c.Total++
			switch v.Outcome {
			case rubric.OutcomePass:
				c.Pass++
			case rubric.OutcomeFail:
				c.Fail++
			default:
				c.NA++
			}
		}
	}
	out := make([]ItemSummary, 0, len(counts))
	for _, it := range rubric.Items() {
		out = append(out, *counts[it])
	}
	return out
}

// TotalFails — число провалов по всему прогону. Ноль = Мастер прошёл рубрику.
func TotalFails(r *Report) int {
	n := 0
	for _, turn := range r.Turns {
		for _, v := range turn.Verdicts {
			if v.Outcome == rubric.OutcomeFail {
				n++
			}
		}
	}
	return n
}

// Markdown — отчёт для человека: сводка, таблица по ходам, список провалов.
//
// Формат таблицы: одна колонка на пункт Эталона. Значки: ✓ pass, ✗ fail, –
// n/a. Провал всегда дублируется в разделе «Провалы» с причиной — по
// одному знаку в таблице причину не восстановить.
func Markdown(r *Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Эталон Мастера — %s\n\n", r.Scenario)
	fmt.Fprintf(&b, "- **Модель:** %s (%s)\n", r.Model, r.Provider)
	fmt.Fprintf(&b, "- **Начало:** %s\n", r.StartedAt.Format("2006-01-02 15:04:05 MST"))
	fmt.Fprintf(&b, "- **Ходов:** %d\n", len(r.Turns))
	fmt.Fprintf(&b, "- **Провалов:** %d\n\n", TotalFails(r))

	// Сводка по пунктам.
	b.WriteString("## Сводка по пунктам\n\n")
	b.WriteString("| Пункт | pass | fail | n/a |\n|---|---:|---:|---:|\n")
	for _, s := range SummarizeReport(r) {
		fmt.Fprintf(&b, "| %s | %d | %d | %d |\n", s.Item, s.Pass, s.Fail, s.NA)
	}

	// Таблица по ходам.
	b.WriteString("\n## По ходам\n\n")
	items := rubric.Items()
	b.WriteString("| # | заявка |")
	for _, it := range items {
		fmt.Fprintf(&b, " %s |", it)
	}
	b.WriteString("\n|---:|---|")
	for range items {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for _, turn := range r.Turns {
		fmt.Fprintf(&b, "| %d | %s |", turn.Index, tableCell(turn.Input))
		byItem := map[rubric.Item]rubric.Verdict{}
		for _, v := range turn.Verdicts {
			byItem[v.Item] = v
		}
		for _, it := range items {
			v, ok := byItem[it]
			if !ok {
				b.WriteString(" – |")
				continue
			}
			fmt.Fprintf(&b, " %s |", mark(v))
		}
		b.WriteString("\n")
	}

	// Пробелы: пункты, которые ПРИМЕНИМЫ, но не оценены (судья не
	// подключён или ответил мусором). Это не «– не применимо», это «не
	// знаем» — и прятать такое нельзя.
	gaps := gapItems(r)
	if len(gaps) > 0 {
		b.WriteString("\n### Пробелы оценки\n\n")
		b.WriteString("Эти пункты применимы к ходам, но не оценены: нет LLM-судьи или модель ответила неразборчиво.\n")
		b.WriteString("Пока они не закрыты, этал неполон.\n\n")
		for _, g := range gaps {
			fmt.Fprintf(&b, "- %s\n", g)
		}
	}

	// Нижняя планка: пункты, которые сработали у Мастера-человека (теги
	// Эталона), но не вытянуты ИИ-Мастером. Это ответ на «держит ли ИИ
	// человеческую линию», а не «много ли он набирает».
	eg := EtalonGaps(r)
	b.WriteString("\n## Нижняя планка (человек-Мастер vs ИИ)\n\n")
	if len(eg) == 0 {
		b.WriteString("ИИ-Мастер вытянул все пункты, которые сработали у Мастера-человека.\n")
	} else {
		b.WriteString("Эти пункты сработали у Мастера-человека, но ИИ-Мастер их не вытянул:\n\n")
		b.WriteString("| Ход | Пункт | У ИИ |\n|---:|---|---|\n")
		for _, g := range eg {
			fmt.Fprintf(&b, "| %d | %s | %s |\n", g.Turn, g.Item, g.State)
		}
	}

	// Провалы с причинами.
	b.WriteString("\n## Провалы\n\n")
	fails := 0
	for _, turn := range r.Turns {
		for _, v := range turn.Verdicts {
			if v.Outcome != rubric.OutcomeFail {
				continue
			}
			fails++
			fmt.Fprintf(&b, "### Ход %d — %s\n\n", turn.Index, v.Item)
			if turn.Input != "" {
				fmt.Fprintf(&b, "Заявка: %s\n\n", turn.Input)
			}
			fmt.Fprintf(&b, "- %s\n\n", v.Reason)
		}
	}
	if fails == 0 {
		b.WriteString("Провалов нет.\n")
	}

	// Прозы — чтобы отчёт был самодостаточным: видно, ЧТО именно оценено.
	b.WriteString("\n## Прозы Мастера\n\n")
	for _, turn := range r.Turns {
		fmt.Fprintf(&b, "### Ход %d\n\n", turn.Index)
		if turn.Input != "" {
			fmt.Fprintf(&b, "**Заявка:** %s\n\n", turn.Input)
		}
		if turn.Note != "" {
			fmt.Fprintf(&b, "**Референс Эталона:** %s\n\n", turn.Note)
		}
		if turn.Guard != "" {
			fmt.Fprintf(&b, "**Страж:** %s\n\n", turn.Guard)
		}
		if turn.Prose == "" {
			b.WriteString("_(прозы нет)_\n\n")
			continue
		}
		fmt.Fprintf(&b, "%s\n\n", turn.Prose)
	}

	return b.String()
}

// mdWithoutProse — тот же отчёт, но без раздела «Прозы Мастера». Для CI,
// где важен вердикт, а не текст.
func mdWithoutProse(r *Report) string {
	full := Markdown(r)
	const marker = "\n## Прозы Мастера\n"
	if i := strings.Index(full, marker); i >= 0 {
		return full[:i]
	}
	return full
}

// JSONL — построчная выгрузка для CI и для последующего анализа. Одна
// строка на ход: полный набор вердиктов + проза. Файл читается потоково,
// поэтому не нужен целиком в памяти.
func JSONL(r *Report) ([]byte, error) {
	var b strings.Builder
	for _, turn := range r.Turns {
		line, err := json.Marshal(turn)
		if err != nil {
			return nil, err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

// mark — значок вердикта.
//
//	✓ — pass
//	✗ — fail
//	– — не применимо к ходу (нет броска и т. п.): правильный NA.
//	? — применимо, но НЕ ОЦЕНЕНО (нет LLM-судьи / модель ответила мусором).
//	    Это пробел, и его видно сразу — иначе отчёт врал бы, что «всё хорошо».
func mark(v rubric.Verdict) string {
	switch v.Outcome {
	case rubric.OutcomePass:
		return "✓"
	case rubric.OutcomeFail:
		return "✗"
	default:
		if v.Source == rubric.KindLLM {
			return "?"
		}
		return "–"
	}
}

// gapItems — пункты, которые остались неоценёнными, хотя применимы.
// Собираются по всему прогону: пункт, не оценённый НИ НА ОДНОМ ходе, —
// пробел.
func gapItems(r *Report) []rubric.Item {
	seen := map[rubric.Item]bool{}
	for _, turn := range r.Turns {
		for _, g := range rubric.GapVerdicts(turn.Verdicts) {
			seen[g] = true
		}
	}
	var out []rubric.Item
	for _, it := range rubric.Items() {
		if seen[it] {
			out = append(out, it)
		}
	}
	return out
}

// tableCell — экранирование для markdown-таблицы: переносы строк ломают
// таблицу, а вертикальная черта ломает колонку.
func tableCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.TrimSpace(s)
	if s == "" {
		return "_(старт)_"
	}
	if len([]rune(s)) > 60 {
		s = string([]rune(s)[:57]) + "…"
	}
	return s
}
