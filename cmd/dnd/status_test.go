package main

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// Шапка полноэкранного режима раньше несла путь к файлу дела (Title:
// *casePath) — он не меняется по ходу игры, и вместо него в шапке место —
// имя текущей локации и номер хода: обе величины меняются по ходу игры и
// доступны прямо в main через game.DB.Locations[game.Node].Name и
// session.Turn(). statusOf обязан собирать их в Status, а не оставлять
// шапку статичной.
func TestStatusOfShowsLocationAndTurn(t *testing.T) {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Name: "Пристань"}
	game := core.NewGame(core.Config{DB: db, Start: "n_quay"})
	session := cli.NewSession(game, strings.NewReader(""), &strings.Builder{})
	session.Feed("look")

	status := statusOf(game, session, nil)()
	if !strings.Contains(status, "Пристань") {
		t.Errorf("статус не показал место: %q", status)
	}
	if !strings.Contains(status, "ход 1") {
		t.Errorf("статус не показал ход: %q", status)
	}
}

// Место меняется, когда игрок переходит между узлами: статус обязан читать
// его на каждый вызов, а не запоминать при создании.
func TestStatusOfReflectsCurrentLocationAfterMove(t *testing.T) {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Name: "Пристань"}
	db.Locations["n_forge"] = store.Location{ID: "n_forge", Name: "Кузня"}
	game := core.NewGame(core.Config{DB: db, Start: "n_quay"})
	session := cli.NewSession(game, strings.NewReader(""), &strings.Builder{})

	status := statusOf(game, session, nil)
	if got := status(); !strings.Contains(got, "Пристань") {
		t.Fatalf("до перехода статус %q не про Пристань", got)
	}

	game.Node = "n_forge"
	if got := status(); !strings.Contains(got, "Кузня") {
		t.Errorf("после перехода статус %q не обновился на Кузню", got)
	}
}
