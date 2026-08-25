package core

import "github.com/kliuchnikovv/dnd/store"

// Знание мест. Гейт перемещения стоит здесь, и он один: место известно или
// неизвестно, «разрешено» как понятие не существует.
//
// Почему знание, а не разрешение: игрок, которому только что объяснили дорогу,
// не должен получать отказ. Пока гейт стоял на разрешении, рассказ персонажа
// не менял мира — импровизация оставалась украшением.

// KnowsPlace — знает ли парти это место.
func (g *Game) KnowsPlace(n store.NodeID) bool {
	return g.DB.KnowsPlace(g.party, g.CaseID, n)
}

// knowPlace помечает место известным и отвечает, случилось ли это впервые.
//
// Неэкспортирован намеренно: снаружи домена место делает известным только
// ProposeMutation, где проверяется смежность. Экспортированный писатель был бы
// второй дверью — без проверки.
func (g *Game) knowPlace(n store.NodeID) bool {
	if g.KnowsPlace(n) {
		return false
	}
	g.DB.KnowPlace(g.party, g.CaseID, n)
	return true
}

// KnownPlaces — известные места этого дела в стабильном порядке.
func (g *Game) KnownPlaces() []store.NodeID {
	return g.DB.PlacesOf(g.party, g.CaseID)
}
