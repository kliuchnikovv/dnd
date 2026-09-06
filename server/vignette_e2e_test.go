package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/master"
	"github.com/kliuchnikovv/dnd/scenegen"
	"github.com/kliuchnikovv/dnd/store"
	"github.com/kliuchnikovv/dnd/vignette"
)

// Секрет и глубокий (закрытый) тир — с уникальными маркерами, чтобы их утечку в
// нарратив было видно точно.
const (
	secretTruth = "СЕКРЕТ-ЗА-ДВЕРЬЮ-НЕ-ЧЕЛОВЕК-УПЫРЬ"
	deepTier    = "ГЛУБОКИЙ-ТИР-ОНО-ЗОВЁТ-ПО-ИМЕНИ"
	doorTell    = "Голос знаком, будто соседа"
	stormTell   = "Буря уже перевалила за середину"
)

func holdSpec() *scenegen.SceneSpec {
	return &scenegen.SceneSpec{
		Title: "Гость к ночи", Intro: "Ты пережидаешь бурю в пустом хуторе.",
		Truth: secretTruth,
		Mode:  "hold", Ambient: "Ночь, буря воет за ставнями.", SafeNote: "Ты у очага, дверь на засове.",
		WinText: "Рассвет — ты достоял, дверь так и не открыл.", LoseText: "Ты снимаешь засов — и оно входит.",
		Goal:  3,
		Beats: []string{"Голос за дверью просит впустить.", "Голос делается ласков.", "Голос твердеет и грозит."},
		Objects: []scenegen.ObjectSpec{
			{ID: "door", Name: "дверь", Surface: "Дверь на засове; в неё стучат.", Aspect: "listen",
				Tiers: []scenegen.TierSpec{
					{Text: doorTell, Passive: 10},
					{Text: deepTier, Passive: 0, Grants: "wrong"}, // активный: откроется лишь на хорошем броске
				}},
			{ID: "storm", Name: "буря", Surface: "За ставнями воет буря.", Aspect: "look",
				Tiers: []scenegen.TierSpec{{Text: stormTell, Passive: 10}}},
		},
	}
}

// echoNarrator — Мастер на fake-шлюзе, ЭХО-ответ: возвращает СВОЙ ВХОД дословно.
// Так нарратив = ровно то, что ушло в нарратор, и тест стережёт саму проводку:
// если в него просочится правда/закрытый тир — они всплывут в выводе.
func echoNarrator() *master.Master {
	f := llm.NewFake("fake", true).ReplyWith(func(r llm.Request) string { return r.Input })
	gw := llm.NewGateway(
		llm.NewRouter().Route(llm.RoleNarrator, llm.Target{Provider: f, Model: "claude-haiku-4-5"}),
		llm.NewLedger(llm.Caps{}))
	return master.New(gw)
}

// Главный тест среза: сгенерированная сцена проходится через сервер end-to-end,
// доходит до победы hold, и НИ scene.Truth, НИ текст закрытого тира не попадают в
// нарратив — Мастеру ушло только revealed (анти-лик конструкцией, ADR-0008/0009).
func TestVignetteTrackPlaysAndNeverLeaks(t *testing.T) {
	spec := holdSpec()
	scenegen.Repair(spec)
	if errs := scenegen.Validate(spec); len(errs) > 0 {
		t.Fatalf("фикстура невалидна: %v", errs)
	}
	sc := vignette.FromSpec(spec)
	// Fixed(1): активные броски проваливаются → глубокий тир НИКОГДА не
	// открывается (остаётся закрытым), а пассивные tells всё равно доступны.
	st := vignette.NewState(dice.Fixed(1))
	rt := newVignetteRuntime("vig-test", "u", 1, NewMemStore(), echoNarrator(), sc, st)

	sub := rt.subscribe()
	inputs := []string{"прислушиваюсь к двери", "осматриваю бурю", "жду у очага"}
	var prose strings.Builder
	var lastView vignetteView
	for i, text := range inputs {
		ok, msg := rt.applyInput(context.Background(), i+1, inputPayload{Text: text})
		if !ok {
			t.Fatalf("ход %q не применился: %s", text, msg)
		}
		for _, f := range collectUntilDone(t, sub) {
			switch f.Op {
			case OpMessage:
				if f.Kind == KindData {
					var d textDelta
					_ = json.Unmarshal(f.Payload, &d)
					prose.WriteString(d.Delta + " ")
				}
			case OpSessionState:
				_ = json.Unmarshal(f.Payload, &lastView)
			}
		}
	}

	// 1) Сцена доиграна до победы hold.
	if !lastView.Ended || lastView.EndText != spec.WinText {
		t.Fatalf("hold не дошёл до победы через сервер: ended=%v end=%q", lastView.Ended, lastView.EndText)
	}
	// 2) Нарратив был (тест не вакуумный): пассивный tell двери прозвучал.
	narr := prose.String()
	if !strings.Contains(narr, doorTell) {
		t.Fatalf("нарратив пуст/без revealed — проводка не сработала:\n%s", narr)
	}
	// 3) ГЛАВНОЕ: ни правды, ни закрытого тира в нарративе.
	if strings.Contains(narr, secretTruth) {
		t.Fatalf("УТЕЧКА: scene.Truth просочилась в нарратив:\n%s", narr)
	}
	if strings.Contains(narr, deepTier) {
		t.Fatalf("УТЕЧКА: текст ЗАКРЫТОГО тира просочился в нарратив:\n%s", narr)
	}
}

// Полный путь через сервер: POST /sessions с виньетка-героем и виньетка-делом
// (cases/nightguest) поднимает виньетка-сессию, и в неё можно сыграть ход.
func TestVignetteCreateOverHTTP(t *testing.T) {
	cs := NewCharacterStore()
	cs.Save(&store.Character{ID: "vig-hero", Ruleset: VignetteRulesKind})
	m := NewManager(casesRoot)
	srv := New(m, WithCharacters(cs))

	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"case":"nightguest","seed":1,"character_id":"vig-hero"}`)
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/sessions", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /sessions виньетки: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("ответ не разобран: %v", err)
	}
	chatID := resp["chat_id"]
	if chatID == "" {
		t.Fatalf("нет chat_id в ответе: %s", rec.Body.String())
	}

	rt, ok := m.GetVignette(chatID)
	if !ok {
		t.Fatalf("виньетка-сессия не поднялась: %s", chatID)
	}
	// Один ход проходит через живой рантайм.
	if ok, msg := rt.applyInput(context.Background(), 1, inputPayload{Text: "прислушиваюсь к двери"}); !ok {
		t.Fatalf("ход в виньетке через сервер не прошёл: %s", msg)
	}
}

// CreateVignette прогоняет спек через repair+validate+map и поднимает живую
// сессию в sibling-карте; битую сцену не пускает.
func TestCreateVignetteBuildsSession(t *testing.T) {
	m := NewManager(casesRoot)
	id, err := m.CreateVignette(holdSpec(), 1, "u")
	if err != nil {
		t.Fatalf("CreateVignette: %v", err)
	}
	if _, ok := m.GetVignette(id); !ok {
		t.Fatalf("сессия виньетки не зарегистрирована: %s", id)
	}
	// Битая сцена (disable без win_target/hazard) — отказ.
	broken := &scenegen.SceneSpec{Title: "x", Intro: "x", Truth: "x", Ambient: "x", SafeNote: "x", WinText: "x", Mode: "disable"}
	if _, err := m.CreateVignette(broken, 1, "u"); err == nil {
		t.Fatal("битая сцена прошла в сессию — валидатор не сработал")
	}
}
