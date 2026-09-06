package vignette

import (
	"encoding/json"
	"io"
)

// Recorder пишет прогон виньетки в JSONL — для офлайн-оценки утечек Мастера.
// Шапка несёт КЛЮЧ-ОТВЕТ (правду сцены и все тиры), дальше — по строке на ход:
// что ввёл игрок, как понял судья, честный бросок, ЧТО ядро отдало (revealed) и
// ЧТО написал Мастер. Сравнение «дали vs сказал» — и есть проверка анти-утечки.
// (Порт proto/recorder.go; сам «живой» прогон на LLM — Ф6, с ключом.)
type Recorder struct {
	enc *json.Encoder
	n   int
}

// NewRecorder создаёт рекордер и пишет шапку-ключ (правда + тиры сцены).
func NewRecorder(w io.Writer, sc *Scene) *Recorder {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{
		"type":  "header",
		"title": sc.Title,
		"mode":  sc.Mode,
		"truth": sc.Truth,
		"scene": sceneDump(sc),
		"note":  "eval: сравнить revealed (что дало ядро) с master_output (что сказал Мастер); утечка = суть, которой не было в revealed",
	})
	return &Recorder{enc: enc}
}

// Turn пишет одну строку хода. masterOutput — итоговая проза Мастера (может быть
// пустой в офлайне).
func (r *Recorder) Turn(input string, ruling Ruling, res Result, masterOutput string) {
	if r == nil {
		return
	}
	r.n++
	rec := map[string]any{
		"type":          "turn",
		"n":             r.n,
		"input":         input,
		"ruling":        ruling,
		"revealed":      res.Revealed,
		"state_note":    res.StateNote,
		"beat":          res.Beat,
		"ended":         res.Ended,
		"end_text":      res.EndText,
		"master_output": masterOutput,
	}
	if res.Roll != nil {
		rec["roll"] = map[string]int{
			"d20": res.Roll.D20, "mod": res.Roll.Mod, "dc": res.Roll.DC,
			"total": res.Roll.Total, "margin": res.Roll.Margin,
		}
	}
	_ = r.enc.Encode(rec)
}

// sceneDump — сериализуемый ключ-ответ сцены (правда + все объекты/тиры) для шапки.
func sceneDump(sc *Scene) []map[string]any {
	objs := make([]map[string]any, 0, len(sc.Objects))
	for _, id := range allObjectIDs(sc) {
		o := sc.Objects[id]
		aspects := map[string][]map[string]any{}
		for name, a := range o.Aspects {
			tiers := make([]map[string]any, 0, len(a.Tiers))
			for _, t := range a.Tiers {
				tiers = append(tiers, map[string]any{"text": t.Text, "passive": t.Passive, "grants": t.Grants})
			}
			aspects[name] = tiers
		}
		objs = append(objs, map[string]any{"id": o.ID, "surface": o.Surface, "hidden": o.Hidden, "aspects": aspects})
	}
	return objs
}
