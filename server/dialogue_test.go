package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kliuchnikovv/dnd/store"
	"github.com/kliuchnikovv/dnd/view"
)

// npcTalkToken находит токен разговорного хода — варианта, целящего в NPC.
func npcTalkToken(t *testing.T, rt *sessionRuntime) string {
	t.Helper()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for _, a := range rt.game.Affordances(rt.spokenTo) {
		if e, ok := rt.game.DB.Entities[a.Intent.Args.Target]; ok && e.Kind == store.EntityNPC {
			return view.OptionToken(a)
		}
	}
	t.Fatal("в деле нет разговорного хода к NPC")
	return ""
}

// Разговорный ход даёт два сегмента: обрамление (gm) и прямую речь NPC (npc со
// speaker). Клиент видит два OpStart с ролями по порядку, а лента хранит обе
// записи раздельно.
func TestTalkTurnStreamsFramingThenReply(t *testing.T) {
	m := narratorManager(t, "реплика")
	id, _ := m.Create("harbour", 1, "u")
	rt, _ := m.Get(id)
	waitOpeningDone(t, rt) // отделяем опенинг от хода

	sub := rt.subscribe()
	tok := npcTalkToken(t, rt)
	ok, msg := rt.applyInput(context.Background(), 1, inputPayload{Token: tok})
	if !ok {
		t.Fatalf("ход не применился: %s", msg)
	}
	frames := collectUntilDone(t, sub)

	var roles []string
	var npcSpeaker string
	for _, f := range frames {
		if f.Op == OpStart {
			var ps proseStart
			if err := json.Unmarshal(f.Payload, &ps); err != nil {
				t.Fatalf("OpStart payload: %v", err)
			}
			roles = append(roles, ps.Role)
			if ps.Role == RoleNPC {
				npcSpeaker = ps.Speaker
			}
		}
	}
	if len(roles) != 2 || roles[0] != RoleGM || roles[1] != RoleNPC {
		t.Fatalf("ждали OpStart[gm, npc], получили %v", roles)
	}
	if npcSpeaker == "" {
		t.Fatalf("реплика NPC без имени говорящего")
	}

	// Лента: player + обрамление gm + реплика npc.
	entries, _ := rt.store.Transcript(context.Background(), store.SessionID(id))
	var roleSeq []string
	for _, e := range entries {
		roleSeq = append(roleSeq, e.Role)
	}
	// Последние три записи — этот ход (до них мог лечь опенинг gm).
	if n := len(roleSeq); n < 3 || roleSeq[n-3] != RolePlayer || roleSeq[n-2] != RoleGM || roleSeq[n-1] != RoleNPC {
		t.Fatalf("лента хода = %v, ждали ...[player, gm, npc]", roleSeq)
	}
}
