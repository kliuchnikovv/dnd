package cli

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/view"
)

// TestTokenInputResolvesLikeNumber — токен опции и её номер это один путь
// применения: сессия разворачивает токен в тот же интент, что и номер, и гонит
// его тем же applyIntent. Расхождение вывода означало бы второй путь к одному
// ходу — ровно то, чего единый apply избегает.
func TestTokenInputResolvesLikeNumber(t *testing.T) {
	var outNum strings.Builder
	sNum := NewSession(renderGame(t), strings.NewReader(""), &outNum).WithAffordances()
	sNum.Start()

	var outTok strings.Builder
	sTok := NewSession(renderGame(t), strings.NewReader(""), &outTok).WithAffordances()
	sTok.Start()

	offered := sTok.Offered()
	if len(offered) == 0 {
		t.Fatal("на старте нет опций")
	}
	token := view.OptionToken(offered[0])

	sNum.Feed("1")
	sTok.Feed(token)

	if outNum.String() != outTok.String() {
		t.Errorf("токен и номер разошлись:\n num=%q\n tok=%q", outNum.String(), outTok.String())
	}
}
