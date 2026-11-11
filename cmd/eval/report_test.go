package main

// Тесты отчёта: сводка, таблица, JSONL. Без сети и без файлов — чистые
// функции.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kliuchnikovv/dnd/eval/rubric"
)

func sampleReport() *Report {
	turns := rubric.GoldenRun()
	rep := &Report{
		Model:     "test/model",
		Provider:  "openrouter",
		Scenario:  "«Тихий Дол»",
		StartedAt: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC),
	}
	for i, turn := range turns {
		rec := turn.ToRecord("бросок: d20=16+0 = 16 против 14 · +2. Следы обрываются.")
		vs := rubric.NewRubric().Apply(context.Background(), rec)
		rep.Turns = append(rep.Turns, TurnResult{
			Index:    i + 1,
			Input:    turn.Input,
			Note:     turn.Note,
			Prose:    rec.MasterOutput,
			Etalon:   turn.EtalonTags,
			Verdicts: vs,
		})
	}
	return rep
}

func TestSummarizeReportCoversAllItems(t *testing.T) {
	rep := sampleReport()
	sums := SummarizeReport(rep)
	if len(sums) != len(rubric.Items()) {
		t.Fatalf("сводка на %d пунктов, ожидалось %d", len(sums), len(rubric.Items()))
	}
	for i, s := range sums {
		if s.Item != rubric.Items()[i] {
			t.Errorf("позиция %d: %s вместо %s", i, s.Item, rubric.Items()[i])
		}
	}
}

func TestMarkdownHasTableAndSummary(t *testing.T) {
	rep := sampleReport()
	md := Markdown(rep)
	for _, want := range []string{
		"# Эталон Мастера",
		"## Сводка по пунктам",
		"## По ходам",
		"## Провалы",
		"## Прозы Мастера",
		"| Пункт | pass | fail | n/a |",
		"test/model",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("в отчёте нет %q", want)
		}
	}
	// Каждый ход — строка таблицы.
	for i := 1; i <= len(rep.Turns); i++ {
		if !strings.Contains(md, "| "+itoa(i)+" |") {
			t.Errorf("в таблице нет хода %d", i)
		}
	}
}

func TestMarkdownWithoutProseDropsSection(t *testing.T) {
	rep := sampleReport()
	md := mdWithoutProse(rep)
	if strings.Contains(md, "## Прозы Мастера") {
		t.Error("quiet-режим не убрал прозы")
	}
	if !strings.Contains(md, "## Сводка по пунктам") {
		t.Error("quiet-режим убрал сводку")
	}
}

func TestJSONLOneLinePerTurn(t *testing.T) {
	rep := sampleReport()
	data, err := JSONL(rep)
	if err != nil {
		t.Fatalf("JSONL: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != len(rep.Turns) {
		t.Fatalf("строк %d, ожидалось %d", len(lines), len(rep.Turns))
	}
	for i, line := range lines {
		var tr TurnResult
		if err := json.Unmarshal([]byte(line), &tr); err != nil {
			t.Fatalf("строка %d не разобралась: %v", i+1, err)
		}
		if tr.Index != i+1 {
			t.Errorf("строка %d: index=%d", i+1, tr.Index)
		}
		if len(tr.Verdicts) != len(rubric.Items()) {
			t.Errorf("строка %d: вердиктов %d, ожидалось %d", i+1, len(tr.Verdicts), len(rubric.Items()))
		}
	}
}

func TestTableCellEscapesPipe(t *testing.T) {
	got := tableCell("а | б")
	if strings.Contains(got, " | ") {
		t.Errorf("вертикальная черта не экранирована: %q", got)
	}
	if !strings.Contains(got, "\\|") {
		t.Errorf("нет экранирования: %q", got)
	}
}

func TestTotalFailsCountsOnlyFails(t *testing.T) {
	rep := sampleReport()
	want := TotalFails(rep)
	var manual int
	for _, turn := range rep.Turns {
		for _, v := range turn.Verdicts {
			if v.Outcome == rubric.OutcomeFail {
				manual++
			}
		}
	}
	if want != manual {
		t.Errorf("TotalFails=%d, вручную=%d", want, manual)
	}
}

// EtalonGaps: пункт, который человек-Мастер вытянул, а ИИ — нет, должен
// попасть в список. Пункт, который оба вытянули, — не должен.
func TestEtalonGaps(t *testing.T) {
	rep := &Report{Turns: []TurnResult{{
		Index:  1,
		Etalon: []rubric.Item{rubric.A2, rubric.A3},
		Verdicts: []rubric.Verdict{
			{Item: rubric.A2, Outcome: rubric.OutcomePass, Source: rubric.KindAuto},
			{Item: rubric.A3, Outcome: rubric.OutcomeFail, Source: rubric.KindAuto},
		},
	}}}
	gaps := EtalonGaps(rep)
	if len(gaps) != 1 {
		t.Fatalf("gaps=%d, ожидался 1: %+v", len(gaps), gaps)
	}
	if gaps[0].Item != rubric.A3 || gaps[0].State != "fail" {
		t.Errorf("gap=%+v, ожидался A3/fail", gaps[0])
	}
}

func TestEtalonGapsCountsNA(t *testing.T) {
	rep := &Report{Turns: []TurnResult{{
		Index:  2,
		Etalon: []rubric.Item{rubric.B5},
		Verdicts: []rubric.Verdict{
			{Item: rubric.B5, Outcome: rubric.OutcomeNA, Source: rubric.KindLLM},
		},
	}}}
	gaps := EtalonGaps(rep)
	if len(gaps) != 1 || gaps[0].State != "na" {
		t.Errorf("ожидался один gap со state=na, получено %+v", gaps)
	}
}

// В отчёте должен быть блок сравнения с человеком-Мастером.
func TestMarkdownHasEtalonSection(t *testing.T) {
	md := Markdown(sampleReport())
	if !strings.Contains(md, "Нижняя планка") {
		t.Error("в отчёте нет блока «Нижняя планка»")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
