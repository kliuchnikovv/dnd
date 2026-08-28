package intent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
)

// chatParserWith — парсер чат-режима на фейковом провайдере. Роль своя, и
// маршрут обязан быть проложен по ней: чат-режим отвечает и разбором, и
// репликой, значит и тиринг у него свой.
func chatParserWith(t *testing.T, replyJSON string) (*Parser, *llm.Fake) {
	t.Helper()
	f := llm.NewFake("fake", true).ReplyWith(func(llm.Request) string { return replyJSON })
	gw := llm.NewGateway(
		llm.NewRouter().Route(llm.RoleChatMaster, llm.Target{Provider: f, Model: "claude-haiku-4-5"}),
		llm.NewLedger(llm.Caps{}))
	return NewChatParser(gw), f
}

// Один вызов даёт и разбор, и реплику: в этом весь чат-режим. Два вызова —
// это то, что уже работает под -nl.
func TestChatReturnsIntentAndReplyInOneCall(t *testing.T) {
	p, f := chatParserWith(t, `{"outcome":"intent","verb":"examine","target":"p_barrels",`+
		`"topic":"","item":"","reply":"Вы наклоняетесь к штабелю бочек."}`)

	got, err := p.Parse(context.Background(), "осмотреть бочки", harbourHint(t), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Accepted() {
		t.Fatalf("разбор не принят: %+v", got)
	}
	if !strings.Contains(got.Reply, "штабелю бочек") {
		t.Errorf("реплика не доехала: %q", got.Reply)
	}
	if len(f.Calls()) != 1 {
		t.Errorf("вызовов %d, ждали один — в этом смысл чат-режима", len(f.Calls()))
	}
	if f.Calls()[0].Role != llm.RoleChatMaster {
		t.Errorf("чат-режим вызван чужой ролью: %q", f.Calls()[0].Role)
	}
}

// Реплика необязательна: модель промолчала — ход всё равно идёт, исход
// печатает ядро. Надстройка не должна быть условием работы.
func TestChatSurvivesMissingReply(t *testing.T) {
	p, _ := chatParserWith(t, `{"outcome":"intent","verb":"examine","target":"p_barrels",`+
		`"topic":"","item":""}`)
	got, err := p.Parse(context.Background(), "осмотреть бочки", harbourHint(t), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Accepted() {
		t.Fatalf("отсутствие реплики сорвало разбор: %+v", got)
	}
	if got.Reply != "" {
		t.Errorf("реплика взялась из ниоткуда: %q", got.Reply)
	}
}

// Проверка значений в чат-режиме та же: ссылка на то, чего в сцене нет,
// отклоняется, и никакая реплика её не оправдывает.
func TestChatRejectsReferencesOutsideTheScene(t *testing.T) {
	p, _ := chatParserWith(t, `{"outcome":"intent","verb":"question","target":"e_призрак",`+
		`"topic":"","item":"","reply":"Вы поворачиваетесь к нему."}`)
	got, err := p.Parse(context.Background(), "спросить призрака", harbourHint(t), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Accepted() {
		t.Errorf("сущность вне сцены пропущена: %+v", got.Intent)
	}
}

// Схема чат-режима требует реплику: без обязательности модель её не заполняет
// — на этом уже обжигались с target, topic и item (docs/status.md §3.5).
func TestChatSchemaRequiresReply(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(chatSchemaJSONFor(harbourHint(t))), &schema); err != nil {
		t.Fatal(err)
	}
	props := schema["properties"].(map[string]any)
	if _, ok := props["reply"]; !ok {
		t.Fatal("в схеме чат-режима нет поля reply")
	}
	var found bool
	for _, r := range schema["required"].([]any) {
		if r == "reply" {
			found = true
		}
	}
	if !found {
		t.Errorf("reply не объявлен обязательным: %v", schema["required"])
	}
}

// Обычный разбор реплики не просит и просить не должен: его схема — контракт,
// и лишнее поле в ней означало бы, что модель платит выводом за то, чего
// никто не покажет.
func TestPlainSchemaHasNoReply(t *testing.T) {
	props := SchemaFor(harbourHint(t))["properties"].(map[string]any)
	if _, ok := props["reply"]; ok {
		t.Error("поле reply просочилось в схему обычного разбора")
	}
}

// Реплика обязана быть безоценочной: об исходе она не знает, потому что
// бросок ещё не сделан. Запрет живёт в промпте — единственном месте, где его
// можно поставить, — и промпт обязан его нести.
func TestChatPromptForbidsClaimingTheOutcome(t *testing.T) {
	p, f := chatParserWith(t, `{"outcome":"intent","verb":"examine","target":"p_barrels",`+
		`"topic":"","item":"","reply":"Вы наклоняетесь."}`)
	if _, err := p.Parse(context.Background(), "осмотреть бочки", harbourHint(t), llm.Request{}); err != nil {
		t.Fatal(err)
	}
	system := f.Calls()[0].System
	for _, must := range []string{"исход", "не утверждай"} {
		if !strings.Contains(strings.ToLower(system), must) {
			t.Errorf("в промпте чат-режима нет запрета про %q:\n%s", must, system)
		}
	}
}

// Роль чат-режима мутирует состояние: её выход несёт интент. Провайдер без
// гарантии схемы к ней не допускается — «почти валидный» JSON мутирует канон.
func TestChatRoleMutatesState(t *testing.T) {
	if !llm.RoleChatMaster.MutatesState() {
		t.Error("роль чат-режима не объявлена мутирующей состояние")
	}
}

// Проба приземляется и в чат-режиме, тем же одним вызовом. Второй разбор для
// неё не заводится: правила у режимов одни, и разойтись им негде.
func TestChatLandsAProbe(t *testing.T) {
	p, f := chatParserWith(t, `{"outcome":"free_probe","probe":"принюхивается к бочкам",`+
		`"target":"","topic":"","item":"","reply":"Вы наклоняетесь к сырой клёпке."}`)
	gi := &GameInterpreter{Parser: p, Game: interpGame(t)}

	in, reply, probe, clarify, err := gi.InterpretChat(context.Background(),
		"чем тут пахнет за бочками", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if in != nil {
		t.Fatalf("проба стала действием: %+v", in)
	}
	if probe != "принюхивается к бочкам" {
		t.Errorf("проба не доехала: %q", probe)
	}
	if clarify != "" {
		t.Errorf("проба пришла уточнением: %q", clarify)
	}
	if reply == "" {
		t.Error("реплика чат-режима потерялась на пробе")
	}
	if len(f.Calls()) != 1 {
		t.Errorf("вызовов %d, ждали один", len(f.Calls()))
	}
}

// Схема чат-режима обязана называть пробу так же, как обычная: разойдись
// перечисления, один режим приземлял бы ввод, а другой отбивал.
func TestChatSchemaOffersFreeProbeToo(t *testing.T) {
	p, f := chatParserWith(t, `{"outcome":"intent","verb":"look","target":"","topic":"","item":"","reply":"x"}`)
	if _, err := p.Parse(context.Background(), "осмотреться", harbourHint(t), llm.Request{}); err != nil {
		t.Fatal(err)
	}
	if schema := f.Calls()[0].Schema; !strings.Contains(schema, OutcomeFreeProbe) {
		t.Errorf("в схеме чат-режима нет пробы:\n%s", schema)
	}
}
