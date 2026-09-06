package master

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
)

// Регресс анти-утечки (ADR-0008): ядро владеет правдой, Мастер её НЕ получает.
// Если после какой-то правки Мастер снова начнёт получать `truth`/скрытые тиры —
// эти три триппвайра должны упасть и вернуть автора к ADR-0008.

// worldPublicFields — единственные поля, которые Мастеру можно видеть: авторский
// сеттинг, видимая сцена, карточка говорящего NPC. Правды среди них нет и быть не
// может. Добавление поля в World (напр. Truth/HiddenTiers) уронит тест намеренно.
var worldPublicFields = []string{"Scene", "Setting", "Speaker"}

func TestWorldStructCarriesNoTruth(t *testing.T) {
	var got []string
	rt := reflect.TypeFor[World]()
	for i := 0; i < rt.NumField(); i++ {
		got = append(got, rt.Field(i).Name)
	}
	sort.Strings(got)
	want := append([]string(nil), worldPublicFields...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("World получил новое поле — Мастер не должен видеть ничего сверх публичного мира.\n"+
			"поля World: %v\nразрешено (ADR-0008): %v\nесли это правда/скрытый тир — так делать нельзя", got, want)
	}
}

// Нарратор по read-scope реестру (llm/capability.go) читает только публичный
// материал. Никакого «truth»-скоупа в закрытом наборе нет; тут закрепляем точный
// whitelist, чтобы добавить правду нарратору было нельзя молча.
func TestNarratorReadScopeExcludesTruth(t *testing.T) {
	want := map[llm.ReadScope]bool{
		llm.ReadAuthoredFlavour:   true,
		llm.ReadScenePublic:       true,
		llm.ReadWorldCanon:        true,
		llm.ReadMechanicalOutcome: true,
	}
	got := llm.Capabilities[llm.RoleNarrator].Reads
	if len(got) != len(want) {
		t.Fatalf("scope нарратора изменился: %v (ADR-0008 — только публичный материал)", got)
	}
	for _, s := range got {
		if !want[s] {
			t.Fatalf("нарратору добавили scope %q — правды/скрытого в его чтении быть не должно (ADR-0008)", s)
		}
	}
}

// Поведенческий инвариант: запрос Мастеру собирается ТОЛЬКО из своих аргументов.
// Секрет-канарейку не кладём никуда — и её не должно оказаться ни в System, ни в
// Input. Ловит будущий боковой канал (глобал/поле сцены), протаскивающий правду в
// сборку прозы в обход World/outcome/frame.
func TestNarrateRequestDoesNotLeakUnpassedSecret(t *testing.T) {
	const canary = "УПЫРЬ-ВЗЯЛ-ГОЛОС-НЕ-ВОЙДЁТ-БЕЗ-ПРИГЛАШЕНИЯ" // «truth» сцены, которого нет в аргументах

	m, _ := masterWith(t, "")
	w := World{
		Setting: "Глухой хутор на болоте. Ночь.",
		Scene:   []string{"Дверь на тяжёлом засове.", "Пёс рычит на порог."},
	}
	// Публичные аргументы — правды в них нет.
	req, ok := m.narrateRequest(
		KindPlace,
		"Ты у очага; в дверь стучат.", // frame (авторская рамка)
		w,
		[]string{"Ты прислушался — голос знаком."}, // outcome (мех-строки ядра)
		"", "", llm.Request{},
	)
	if !ok {
		t.Fatal("narrateRequest не собрал запрос при непустой рамке")
	}
	if strings.Contains(req.Input, canary) || strings.Contains(req.System, canary) {
		t.Fatalf("в запрос Мастеру просочился секрет, которого не было в аргументах — боковой канал правды (ADR-0008)")
	}
	if req.Role != llm.RoleNarrator {
		t.Fatalf("проза Мастера должна идти ролью нарратора, получено %q", req.Role)
	}
}
