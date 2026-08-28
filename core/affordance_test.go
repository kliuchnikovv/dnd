package core

import (
	"reflect"
	"testing"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// affordGame — узел, где есть всё сразу: человек, известная тема, три детали,
// известное место и носимая бумага. Набор обязан выбирать из этого, а не
// перечислять всё.
func affordGame() *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Name: "Пристань"}
	db.Locations["n_forge"] = store.Location{ID: "n_forge", Name: "Кузница"}
	db.Entities["e_bern"] = store.Entity{ID: "e_bern", Name: "Берн, стражник",
		Kind: store.EntityNPC, Node: "n_quay"}
	db.Entities["e_body"] = store.Entity{ID: "e_body", Name: "Тело Халдена",
		Kind: store.EntityThing, Node: "n_quay"}
	db.Props["n_quay"] = []store.SceneProp{
		{ID: "p_nets", Node: "n_quay", Name: "Ворох сетей", Kind: "clutter"},
		{ID: "p_crates", Node: "n_quay", Name: "Ящики у стены", Kind: "furniture"},
	}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}
	db.Facts["f_known"] = store.Fact{ID: "f_known", Key: "тело нашли на складе"}
	db.Facts["f_secret"] = store.Fact{ID: "f_secret", Key: "гильдейская печать на шнуре"}
	db.Holders["f_secret"] = []store.FactHolder{{
		FactID: "f_secret", HolderID: "e_body", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"examine"}, Threshold: "hard"},
	}}
	db.Items["i_writ"] = store.Item{ID: "i_writ", Name: "Предписание магистрата", Kind: "writ"}
	g := NewGame(Config{
		DB: db, Rules: fixedRules{OutcomeSuccess}, Dice: nilDice{},
		Truth:   accusation.NewTruth("bern", "cord", "night", "debt"),
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
		StartPlaces: []store.NodeID{"n_forge"},
	})
	g.K.Learn("f_known", "e_briefing")
	g.Acquire("i_writ")
	return g
}

// Набор детерминирован: он часть той же правды, что (seed, script). Плавающий
// порядок сделал бы прогон невоспроизводимым, а игроку — меню, которое
// перетасовывается само.
func TestAffordancesAreDeterministic(t *testing.T) {
	g := affordGame()
	first := g.Affordances("")
	second := g.Affordances("")
	if !reflect.DeepEqual(first, second) {
		t.Errorf("два вызова разошлись:\n%+v\n%+v", first, second)
	}
	if len(first) < 2 || len(first) > 4 {
		t.Errorf("в наборе %d вариантов, ожидалось 2–4: %+v", len(first), first)
	}
}

// Ни один вариант не называет того, чего парти не знает. Утечка здесь была бы
// не в формулировке, а в САМОМ НАБОРЕ: список, где одна опция ведёт к разгадке,
// спойлерит безупречными словами (ADR-0003, T2).
func TestAffordancesNameNothingUnknown(t *testing.T) {
	g := affordGame()
	for _, a := range g.Affordances("") {
		if topic := a.Intent.Args.Topic; topic != "" && !g.K.Knows(topic) {
			t.Errorf("в набор попала неизвестная тема %q", topic)
		}
		if node := a.Intent.Args.Node; node != "" && !g.KnowsPlace(node) {
			t.Errorf("в набор попало неизвестное место %q", node)
		}
		if item := a.Intent.Args.Item; item != "" && !g.Carries(store.ItemID(item)) {
			t.Errorf("в набор попал ненесомый предмет %q", item)
		}
	}
}

// Цели осмотра берутся из деталей узла в порядке ID и держателями не
// отбираются. Иначе набор разметил бы, где лежит авторский контент, — та же
// карта решения, которую запрещает камуфляжный инвариант.
func TestExamineTargetsIgnoreHolders(t *testing.T) {
	g := affordGame()
	plain := examineTargetsOf(g.Affordances(""))

	// У сетей появляется держатель — набор от этого меняться не должен.
	g.DB.Holders["f_secret"] = append(g.DB.Holders["f_secret"], store.FactHolder{
		FactID: "f_secret", HolderID: "p_nets", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"search"}, Threshold: "easy"},
	})
	if got := examineTargetsOf(g.Affordances("")); !reflect.DeepEqual(plain, got) {
		t.Errorf("набор пошёл за держателями: %v → %v", plain, got)
	}
}

func examineTargetsOf(list []Affordance) []store.EntityID {
	var out []store.EntityID
	for _, a := range list {
		if a.Intent.Verb == "examine" {
			out = append(out, a.Intent.Args.Target)
		}
	}
	return out
}

// Тег проверки — класс глагола из реестра, и только там, где бросок есть.
// Порога в теге нет намеренно: число живёт в гейте держателя, то есть в данных
// дела, и напечатать его значит разметить авторские цели.
func TestCheckTagComesFromTheVerbRegistry(t *testing.T) {
	for _, a := range affordGame().Affordances("") {
		def, ok := Verbs[a.Intent.Verb]
		if !ok {
			t.Fatalf("вариант несёт глагол вне реестра: %q", a.Intent.Verb)
		}
		switch {
		case rollDecidedElsewhere[a.Intent.Verb]:
			if a.Check != "" {
				t.Errorf("%s: бросок решает не реестр, а тег обещан: %q",
					a.Intent.Verb, a.Check)
			}
		case def.Rolls && a.Check != def.Class:
			t.Errorf("%s: тег %q, класс реестра %q", a.Intent.Verb, a.Check, def.Class)
		case !def.Rolls && a.Check != "":
			t.Errorf("%s броска не требует, а тег есть: %q", a.Intent.Verb, a.Check)
		}
	}
}

// Каждый вариант обязан быть исполнимым: предложить ход, который ядро отклонит,
// значит соврать игроку меню.
func TestEveryAffordanceIsAccepted(t *testing.T) {
	g := affordGame()
	for _, a := range g.Affordances("") {
		if r := g.Check(a.Intent); r.Refused {
			t.Errorf("%s → %q отклонён ядром: %s",
				a.Intent.Verb, a.Intent.Args.Target, r.Refusal)
		}
	}
}

// Бедный узел даёт меньше вариантов, а не выдуманные. Пустой набор — законный
// исход: свободный ввод рядом и равноправен.
func TestPoorNodeGivesFewerAffordances(t *testing.T) {
	db := store.NewDB()
	db.Locations["n_empty"] = store.Location{ID: "n_empty", Name: "Пустырь"}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}
	g := NewGame(Config{
		DB: db, Rules: fixedRules{OutcomeSuccess}, Dice: nilDice{},
		Truth:   accusation.NewTruth("a", "b", "c", "d"),
		Flavour: map[string]string{}, Start: "n_empty", Actor: "pc",
	})
	if got := g.Affordances(""); len(got) != 0 {
		t.Errorf("на пустом узле набор непуст: %+v", got)
	}
}

// Один человек — один вариант. «Заговорить с Берном» и «расспросить Берна»
// рядом читаются как один и тот же ход, написанный дважды: живой прогон получил
// их первыми двумя строками и не увидел разницы. Место в наборе дороже: четыре
// строки на двух присутствующих должны показывать обоих.
func TestOnePersonGivesOneOption(t *testing.T) {
	g := affordGame()
	g.DB.Entities["e_nils"] = store.Entity{ID: "e_nils", Name: "Нильс, посыльный",
		Kind: store.EntityNPC, Node: "n_quay"}

	seen := map[store.EntityID]int{}
	for _, a := range g.Affordances("") {
		if conversational(a.Intent.Verb) {
			seen[a.Intent.Args.Target]++
		}
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("на %s предложено %d социальных вариантов", id, n)
		}
	}
	if len(seen) < 2 {
		t.Errorf("присутствующих двое, а в наборе %d из них: %+v", len(seen), seen)
	}
}

// Незнакомому предлагают заговорить, знакомого — расспросить. Глагол идёт за
// разговором: предлагать «заговорить» тому, с кем идёт беседа, значит начинать
// её заново, а «расспросить» до знакомства — допрос с порога.
func TestSocialVerbFollowsTheConversation(t *testing.T) {
	g := affordGame()
	if got := socialVerbFor(g, "e_bern"); got != "talk_to" {
		t.Errorf("незнакомому предложено %q", got)
	}
	g.D.Remember("e_bern", "добрый день", "и вам", 1)
	if got := socialVerbFor(g, "e_bern"); got != "question" {
		t.Errorf("после разговора предложено %q", got)
	}
}

// conversational — вариант, который заводит или продолжает разговор.
// Предъявление сюда не входит: показать бумагу — отдельный ход с отдельным
// смыслом, и рядом с «заговорить» он не дубль.
func conversational(v Verb) bool { return v == "talk_to" || v == "question" }

func socialVerbFor(g *Game, id store.EntityID) Verb {
	for _, a := range g.Affordances("") {
		if a.Intent.Args.Target == id && conversational(a.Intent.Verb) {
			return a.Intent.Verb
		}
	}
	return ""
}

// В разговоре набор — реплики: три того, что можно сказать, и один выход.
// Разговор половина детектива, и до этой ветки его в списке не было вовсе.
func TestConversationGivesRepliesAndOneExit(t *testing.T) {
	g := affordGame()
	got := g.Affordances("e_bern")
	if len(got) == 0 {
		t.Fatal("в разговоре набор пуст")
	}
	var replies, exits int
	for _, a := range got {
		if a.Reply {
			replies++
			continue
		}
		exits++
	}
	if replies == 0 {
		t.Errorf("в разговоре нет ни одной реплики: %+v", got)
	}
	if replies > 3 {
		t.Errorf("реплик %d — выходу не осталось места", replies)
	}
	if exits != 1 {
		t.Errorf("выходов из разговора %d, ожидался ровно один: %+v", exits, got)
	}
	if last := got[len(got)-1]; last.Reply {
		t.Error("выход не последний — реплики его вытеснили")
	}
}

// Тема реплики — факт с наименьшей подтверждённостью: детектив обходит
// свидетелей ради второго источника шаткой улики. Правило вращается само —
// подтвердил, слабейшим стал другой.
func TestReplyAsksAboutTheLeastCorroboratedFact(t *testing.T) {
	g := affordGame()
	g.K.Learn("f_secret", "e_bern") // второй известный факт, один источник
	g.K.Learn("f_known", "e_nils")  // у f_known теперь два источника

	if got := replyTopic(g.Affordances("e_bern")); got != "f_secret" {
		t.Errorf("реплика спрашивает про %q, а слабее подтверждён f_secret", got)
	}
}

// Тем известно ноль — вопрос открытый, а не выдуманный.
func TestReplyFallsBackToTheOpenQuestion(t *testing.T) {
	g := affordGame()
	// Банк тем пуст: affordGame выдаёт f_known на старте, снимаем его.
	g.DB.Knowledge = nil
	g.K = NewKnowledge(g.DB)

	for _, a := range g.Affordances("e_bern") {
		if a.Intent.Verb == "question" && a.Intent.Args.Topic != "" {
			t.Errorf("без известных тем реплика назвала тему %q", a.Intent.Args.Topic)
		}
	}
}

// Вариант с темой появляется НЕЗАВИСИМО от того, держит ли собеседник факт.
// Это и есть leak-безопасность набора: до снятия отказа в ядре присутствие
// варианта было бы ответом на вопрос, кто что знает.
//
// Два человека в одном узле: один держит факт, другой не держит ничего. Набор
// обязан совпасть.
func TestTopicReplyDoesNotDependOnWhoHoldsIt(t *testing.T) {
	g := affordGame()
	g.DB.Entities["e_knower"] = store.Entity{ID: "e_knower", Name: "Токе, писарь",
		Kind: store.EntityNPC, Node: "n_quay"}
	g.DB.Holders["f_known"] = []store.FactHolder{{
		FactID: "f_known", HolderID: "e_knower", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"question"}, Threshold: "normal"},
	}}

	knower := replyTopic(g.Affordances("e_knower"))
	stranger := replyTopic(g.Affordances("e_bern"))
	if knower == "" {
		t.Fatal("реплика знающему не назвала темы — сравнивать не с чем")
	}
	if knower != stranger {
		t.Errorf("тема реплики зависит от держателя: у знающего %q, у незнающего %q",
			knower, stranger)
	}
}

// Предъявление предлагается один раз на человека: показать ту же бумагу
// второй раз — не ход, а повтор, и сдвига расположения он всё равно не даёт.
func TestPresentIsOfferedUntilShown(t *testing.T) {
	g := affordGame()
	if !offers(g.Affordances("e_bern"), "present") {
		t.Fatal("носимое не предложено к предъявлению")
	}
	g.D.MarkPresented("e_bern", "i_writ")
	if offers(g.Affordances("e_bern"), "present") {
		t.Error("предъявление предложено повторно тому же человеку")
	}
}

// Вне разговора набор прежний: реплик в нём нет.
func TestOutsideConversationThereAreNoReplies(t *testing.T) {
	for _, a := range affordGame().Affordances("") {
		if a.Reply {
			t.Errorf("вне разговора предложена реплика: %+v", a.Intent)
		}
	}
}

// Собеседник, которого нет в этом узле, разговором не считается: он ушёл, и
// говорить с ним не о чем.
func TestAbsentInterlocutorIsNotAConversation(t *testing.T) {
	g := affordGame()
	g.DB.Entities["e_toke"] = store.Entity{ID: "e_toke", Name: "Токе",
		Kind: store.EntityNPC, Node: "n_forge"}
	for _, a := range g.Affordances("e_toke") {
		if a.Reply {
			t.Errorf("предложена реплика ушедшему: %+v", a.Intent)
		}
	}
}

func replyTopic(list []Affordance) store.FactID {
	for _, a := range list {
		if a.Reply && a.Intent.Verb == "question" {
			return a.Intent.Args.Topic
		}
	}
	return ""
}

func offers(list []Affordance, v Verb) bool {
	for _, a := range list {
		if a.Intent.Verb == v {
			return true
		}
	}
	return false
}

// Меню не врёт и в разговоре: каждая реплика доходит до ядра и не отклоняется.
//
// Проверяется ИСПОЛНЕНИЕМ, а не Check, и это не перестраховка. Отказы
// предъявления живут внутри Apply — present проверяет предмет и адресата уже
// в ходе, — поэтому тест на одном Check был бы зелёным ровно там, где меню
// врёт. Каждый вариант исполняется на своей свежей игре: применённый ход
// изменил бы состояние и сбил следующий.
func TestEveryReplyIsAcceptedWhenApplied(t *testing.T) {
	for i, a := range affordGame().Affordances("e_bern") {
		fresh := affordGame()
		if res := fresh.Apply(a.Intent); res.Refused {
			t.Errorf("вариант %d (%s → %q) отклонён ядром: %s",
				i+1, a.Intent.Verb, a.Intent.Args.Target, res.Refusal)
		}
	}
}

// Реплик ровно три, и четвёртой строкой стоит выход. Комментарий к replies
// утверждает именно это — тест держит утверждение, чтобы оно не разошлось с
// кодом молча.
func TestConversationAlwaysGivesThreeRepliesAndTheExit(t *testing.T) {
	got := affordGame().Affordances("e_bern")
	if len(got) != 4 {
		t.Fatalf("вариантов %d, ожидались три реплики и выход: %+v", len(got), got)
	}
	for i, a := range got[:3] {
		if !a.Reply {
			t.Errorf("вариант %d не реплика: %+v", i+1, a.Intent)
		}
	}
	if got[3].Reply {
		t.Error("четвёртой строкой стоит реплика, а не выход")
	}
}
