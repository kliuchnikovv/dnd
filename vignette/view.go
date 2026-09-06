package vignette

// Экспортируемые проекции сцены/состояния для внешнего драйвера (сервер). Наружу
// уходит ТОЛЬКО видимое: поверхности присутствующих объектов и нейтральное
// положение. Ни правды, ни закрытых тиров здесь нет — анти-лик конструкцией.

// Surfaces — поверхности присутствующих объектов (в порядке сцены). Поверхность
// всегда открыта и тайны не несёт.
func (sc *Scene) Surfaces() []string {
	out := make([]string, 0, len(sc.Order))
	for _, id := range sc.Order {
		if o := sc.Objects[id]; o != nil {
			out = append(out, o.Surface)
		}
	}
	return out
}

// JudgeObjects — присутствующие объекты как цели для судьи (id + имя).
func (sc *Scene) JudgeObjects() []JudgeObject {
	out := make([]JudgeObject, 0, len(sc.Order))
	for _, id := range sc.Order {
		if o := sc.Objects[id]; o != nil {
			out = append(out, JudgeObject{ID: o.ID, Name: o.Name})
		}
	}
	return out
}

// Position — нейтральное физическое положение персонажа (что он и так знает).
func Position(sc *Scene, st *State) string { return physicalNote(sc, st) }

// BuildJudgeView собирает вид для судьи из сцены и состояния.
func BuildJudgeView(sc *Scene, st *State) JudgeView {
	return JudgeView{
		Objects:  sc.JudgeObjects(),
		Position: physicalNote(sc, st),
		Ambient:  sc.Ambient,
		OffPath:  st.OffPath,
		Mire:     st.MireDepth,
	}
}
