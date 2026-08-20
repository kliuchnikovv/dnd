package intent

import "github.com/kliuchnikovv/dnd/naming"

// resolveByName ищет в фразе игрока того, кто есть в сцене. Логика вынесена в
// naming: она нужна и разбору команд, и переводу свободного текста, а
// расхождение между двумя входами было бы багом.
func resolveByName(text string, list []Named) (string, bool) {
	candidates := make([]naming.Candidate, 0, len(list))
	for _, n := range list {
		candidates = append(candidates, naming.Candidate{ID: n.ID, Name: n.Name})
	}
	return naming.Resolve(text, candidates)
}
