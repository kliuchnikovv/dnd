package main

import (
	"strings"
	"testing"
)

// Снепшот считается от СОДЕРЖИМОГО дела: правка дела делает записанный журнал
// непереигрываемым, и узнать об этом надо на реплее, а не по разъезжающимся
// фактам посреди прогона.
func TestSnapshotIDFollowsCaseContent(t *testing.T) {
	a := snapshotID("harbour", []byte(`{"id":"harbour"}`), 3)
	b := snapshotID("harbour", []byte(`{"id":"harbour","x":1}`), 3)
	if a == b {
		t.Error("правка дела не изменила снепшот")
	}
	if again := snapshotID("harbour", []byte(`{"id":"harbour"}`), 3); again != a {
		t.Errorf("тот же вход дал другой снепшот: %s и %s", a, again)
	}
}

// Seed — часть начального состояния: то же дело на другом seed это другая
// сессия, и её журнал не является журналом этой.
func TestSnapshotIDFollowsSeed(t *testing.T) {
	if snapshotID("harbour", []byte("x"), 3) == snapshotID("harbour", []byte("x"), 7) {
		t.Error("другой seed дал тот же снепшот")
	}
}

// Читаемая часть оставлена нарочно: снепшот попадает в баг-репорт, и имя дела
// там полезнее одного хеша.
func TestSnapshotIDNamesTheCase(t *testing.T) {
	if got := snapshotID("harbour", []byte("x"), 3); !strings.HasPrefix(got, "harbour@") {
		t.Errorf("снепшот не называет дело: %s", got)
	}
}

// Идентификатор сессии отличает два прогона одного дела на одном seed, но
// снепшот в нём виден: по журналу должно быть понятно, что переигрывать.
func TestSessionIDCarriesItsSnapshot(t *testing.T) {
	snap := snapshotID("harbour", []byte("x"), 3)
	if got := string(sessionID(snap)); !strings.HasPrefix(got, snap+"/") {
		t.Errorf("идентификатор сессии не называет снепшот: %s", got)
	}
}

// Реплей несовместим с другим источником ходов и с моделями. Молчаливый
// приоритет одного из флагов выдал бы другой прогон за записанный.
func TestCheckReplayRejectsWhatContradictsIt(t *testing.T) {
	if err := checkReplay("j.log", "script.txt", false, false); err == nil {
		t.Error("-replay со -script прошёл молча")
	}
	if err := checkReplay("j.log", "", true, false); err == nil {
		t.Error("-replay с -nl прошёл молча")
	}
	if err := checkReplay("j.log", "", false, true); err == nil {
		t.Error("-replay с -chat прошёл молча")
	}
	if err := checkReplay("j.log", "", false, false); err != nil {
		t.Errorf("чистый реплей отбит: %v", err)
	}
	// Без реплея флаги остаются как были: проверка не должна запрещать то,
	// что работало до неё.
	if err := checkReplay("", "script.txt", true, false); err != nil {
		t.Errorf("прогон без реплея отбит: %v", err)
	}
}
