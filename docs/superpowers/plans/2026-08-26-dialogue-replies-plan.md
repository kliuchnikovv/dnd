# Варианты реплик в разговоре. План реализации

> **Для агентов:** ОБЯЗАТЕЛЬНЫЙ СУБ-СКИЛЛ — `superpowers:subagent-driven-development`
> либо `superpowers:executing-plans`. Шаги помечены чекбоксами для отслеживания.

**Цель:** в разговоре набор вариантов показывает, что можно сказать, а не только
что можно сделать — и для этого ядро перестаёт отказывать в вопросе человеку,
который темы не держит.

**Подход:** интенты выбирает код (детерминированно, из read-scope), слова им даёт
Мастер дешёвым тиром по правилу «всё или ничего», выбор по-прежнему уезжает
номером через тот же `Feed`.

**Стек:** Go 1.26, `go test ./...`, без новых зависимостей.

**Спека:** `docs/superpowers/specs/2026-08-26-dialogue-replies-design.md`

## Глобальные ограничения

- `go test ./...` зелёный после каждой задачи; коммит только на зелёном.
- `gofmt -l .` пуст.
- Граница домена цела: `e2e/architecture_test.go`, `e2e/border_test.go`,
  `e2e/capabilities_test.go` зелёные. `core/mutate.go` экспортирует только
  `ProposeMutation` и `Refusal.Refused` — новые экспорты класть в другие файлы.
- В `core/` нельзя упоминать словарь кости: подстроки `d20`, `attribute`,
  `grit +` запрещены даже в комментариях (`TestCoreMentionsNoDiceVocabulary`).
- Набор вариантов не читает `DB.Holders` и `truth` — ни прямо, ни через хелперы.
- Комментарии на русском, объясняют ПОЧЕМУ, а не ЧТО. Сообщения коммитов —
  в стиле репозитория, с `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`.

**Уточнение к спеке, принятое при планировании.** «Разговор идёт» живёт в
`cli.Session.spokenTo`, а не в домене: `Dossiers.Recent` пишет только актёр, и
без моделей он пуст — вывод «с кем говорим» внутри ядра означал бы, что набор
реплик не появляется в прогоне без моделей вовсе. Поэтому собеседник передаётся
параметром: `Affordances(with store.EntityID)`. Детерминизм цел — `spokenTo`
восстанавливается реплеем, потому что реплей идёт тем же `execute`.

---

### Задача 1: Ядро перестаёт отказывать в вопросе

**Файлы:**
- Правка: `core/turn.go:191-202` (блок темы в `validate`)
- Тест: `core/turn_test.go` (новый тест), `core/hunch_test.go:89-118` (две фикстуры)

**Интерфейсы:**
- Производит: поведение — `Apply(Intent{Verb:"question", Target:X, Topic:T})`
  при отсутствии держателя у `X` возвращает `TurnResult{Refused:false}` с
  броском и без `Learned`.

- [ ] **Шаг 1: Написать падающий тест**

В `core/turn_test.go`:

```go
// Спросить человека о том, чего он не знает, — нормальный ход разговора, а не
// невозможное действие. Живой плейтест утыкался в этот отказ десятками раз
// (см. core/hunch_test.go), и чутьё за три прогона не срабатывало ни разу.
func TestQuestionToSomeoneWhoDoesNotKnowIsNotRefused(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.DB.Entities["e_nils"] = store.Entity{
		ID: "e_nils", Kind: store.EntityNPC, Name: "Нильс", Node: "n_quay",
	}
	g.K.Learn("f_open", "e_briefing")

	got := g.Apply(Intent{Verb: "question", Actor: g.Actor,
		Args: Args{Target: "e_nils", Topic: "f_open"}})
	if got.Refused {
		t.Fatalf("вопрос незнающему отклонён: %s", got.Refusal)
	}
	if len(got.Learned) != 0 {
		t.Errorf("незнающий выдал факт: %+v", got.Learned)
	}
}

// Бросок остаётся. Его отсутствие метило бы незнающих — та же логика, по
// которой осмотр инертной детали бросает кость наравне с настоящей целью.
func TestQuestionToSomeoneWhoDoesNotKnowStillRolls(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.DB.Entities["e_nils"] = store.Entity{
		ID: "e_nils", Kind: store.EntityNPC, Name: "Нильс", Node: "n_quay",
	}
	g.K.Learn("f_open", "e_briefing")

	got := g.Apply(Intent{Verb: "question", Actor: g.Actor,
		Args: Args{Target: "e_nils", Topic: "f_open"}})
	if got.Res == nil {
		t.Fatal("ход прошёл без резолва — отсутствие броска выдаёт незнающего")
	}
}

// Защита от угадывания остаётся: спросить о факте, которого парти не знает,
// по-прежнему нельзя. Перечисление схемы для свободного текста строится из
// party_knowledge, но структурированный ввод такое пропустил бы.
func TestQuestionAboutUnknownFactIsStillRefused(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.DB.Entities["e_nils"] = store.Entity{
		ID: "e_nils", Kind: store.EntityNPC, Name: "Нильс", Node: "n_quay",
	}
	got := g.Apply(Intent{Verb: "question", Actor: g.Actor,
		Args: Args{Target: "e_nils", Topic: "f_gated"}})
	if !got.Refused {
		t.Fatal("спросили о неизвестном парти факте — защита от угадывания дырявая")
	}
}
```

- [ ] **Шаг 2: Убедиться, что тест падает**

Запустить: `go test ./core/ -run 'QuestionToSomeone|QuestionAboutUnknown' -v`
Ожидается: два первых теста FAIL с «вопрос незнающему отклонён: здесь об этом
не расскажут»; третий уже проходит.

- [ ] **Шаг 3: Правка `validate`**

`core/turn.go`, заменить блок 191–202 на:

```go
	if in.Args.Topic != "" {
		holder, held := g.holderFor(in)
		// Отсутствие держателя — НЕ отказ: человек просто не знает, и сказать
		// ему об этом — обычный ход разговора. Раньше здесь стояла стена, и
		// живой плейтест бился в неё десятками ходов подряд.
		//
		// Бросок при этом остаётся (его делает Apply): отсутствие броска метило
		// бы незнающих — та же логика, по которой осмотр инертной детали
		// бросает кость наравне с настоящей целью.
		//
		// Банк тем строится из party_knowledge: спросить о неизвестном нельзя —
		// кроме mandatory-фактов, которые выдаются независимо от того, знает
		// ли парти об их существовании заранее.
		if !g.K.Knows(in.Args.Topic) && !(held && holder.Mandatory) {
			return refuse("парти об этом ничего не знает — спрашивать не о чем"), true
		}
	}
```

- [ ] **Шаг 4: Убедиться, что тесты проходят**

Запустить: `go test ./core/ -run 'QuestionToSomeone|QuestionAboutUnknown' -v`
Ожидается: PASS все три.

- [ ] **Шаг 5: Починить две фикстуры чутья**

`core/hunch_test.go` — `TestRefusedProbesCountAsBeingStuck` (строка 89) и
`TestRefusalStillCostsNoTurn` (строка 107) брали фикстурой именно этот отказ.
Заменить его на отказ по отсутствующей цели, смысл тестов сохраняется.

В `TestRefusedProbesCountAsBeingStuck` заменить тело `stuck`:

```go
	// Цель не в этой локации: мир отказывает, ход не тратится. Раньше здесь
	// стоял вопрос человеку, который темы не держит, — но это перестало быть
	// отказом: спросить незнающего теперь можно, и он отвечает, что не знает.
	stuck := Intent{Verb: "question", Actor: g.Actor,
		Args: Args{Target: "e_ivar", Topic: "f_open"}}
```

Перед циклом добавить выдачу темы, иначе отказ придёт по незнанию, а не по цели:

```go
	g.K.Learn("f_open", "e_briefing")
```

В `TestRefusalStillCostsNoTurn` тем же образом заменить `e_nils` на `e_ivar`
и добавить `g.K.Learn("f_open", "e_briefing")` перед снятием `before`.

- [ ] **Шаг 6: Прогон и коммит**

Запустить: `go test ./...`
Ожидается: всё зелёное.

```bash
gofmt -l . && go test ./... && git add -A && git commit -m "$(cat <<'EOF'
feat(core): вопрос незнающему — не отказ, а обычный ход

Тест чутья хранил запись живого плейтеста: агент произвёл десятки отказов
«здесь об этом не расскажут», и подсказка за три прогона не пришла ни разу.
Стену тогда не убрали — научили считать отказы признаком «застрял».

Спросить свидетеля о том, чего он не видел, — нормальный ход разговора, а
не невозможное действие. Отсутствие держателя перестаёт быть отказом.

Бросок остаётся: его отсутствие метило бы незнающих — та же логика, по
которой осмотр инертной детали бросает кость наравне с настоящей целью.
Цена провала прежняя, знает собеседник или нет.

Защита от угадывания разведена и оставлена: спросить о факте, которого
парти не знает, по-прежнему нельзя.

Текст под это уже был написан — общий ключ question в обоих делах говорит
ровно то, что нужно. Отказ всё это время закрывал прозу, сочинённую под
него.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Задача 2: Набор реплик в разговоре

**Файлы:**
- Правка: `core/affordance.go` (сигнатура `Affordances`, ветка разговора)
- Правка: `cli/repl.go` (единственный вызывающий — в `offerAffordances`)
- Тест: `core/affordance_test.go`

**Интерфейсы:**
- Потребляет: `Knowledge.Confidence(store.FactID) float64`,
  `Knowledge.TopicBank() []store.FactID`, `Dossiers.Presented(store.EntityID, store.ItemID) bool`,
  `Game.Carried() []store.Item` — всё уже есть.
- Производит: `func (g *Game) Affordances(with store.EntityID) []Affordance`;
  поле `Affordance.Reply bool` — «это реплика, а не действие».

- [ ] **Шаг 1: Написать падающий тест**

В `core/affordance_test.go`:

```go
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
```

- [ ] **Шаг 2: Убедиться, что тест падает**

Запустить: `go test ./core/ -run 'Conversation|Reply|TopicReply|PresentIsOffered|OutsideConversation|AbsentInterlocutor'`
Ожидается: FAIL со `too many arguments in call to g.Affordances`.

- [ ] **Шаг 3: Реализация**

`core/affordance.go` — добавить поле в `Affordance`:

```go
	// Reply — это реплика, а не действие: то, что игрок СКАЖЕТ. Отдельным
	// полем, а не по классу глагола: question — класс расследования, а
	// осмотр детали на выходе из разговора социальным не станет никогда.
	// Презентация по этому полю решает, брать ли строку в кавычки.
	Reply bool
```

Заменить `func (g *Game) Affordances() []Affordance` на:

```go
// Affordances — набор вариантов из read scope.
//
// with — собеседник, если разговор идёт. Параметром, а не выведенным изнутри:
// память разговора пишет актёр, и без моделей она пуста — набор реплик не
// появлялся бы в прогоне без моделей вовсе. Детерминизм цел: собеседник
// восстанавливается реплеем, потому что реплей идёт тем же путём применения.
func (g *Game) Affordances(with store.EntityID) []Affordance {
	if g.talkingTo(with) {
		return g.replies(with)
	}
	return g.actions()
}

// talkingTo — идёт ли разговор. Ушедший собеседник разговором не считается:
// говорить с тем, кого здесь нет, не о чем.
func (g *Game) talkingTo(with store.EntityID) bool {
	if with == "" {
		return false
	}
	e, ok := g.DB.Entities[with]
	return ok && e.Kind == store.EntityNPC && e.Node == g.Node
}

// replyLimit — сколько реплик показывается. Три, потому что четвёртая строка
// отдана выходу: из разговора должен быть выход одним нажатием, а не только
// словами.
const replyLimit = 3

// replies — что можно сказать этому человеку.
//
// Кандидаты в фиксированном приоритете; берутся первые подходящие. Подходящих
// меньше трёх — строк меньше трёх: вариант, придуманный ради ровного счёта,
// врёт так же, как отклонённый ядром.
func (g *Game) replies(with store.EntityID) []Affordance {
	var out []Affordance
	add := func(in Intent, ok bool) {
		if !ok || len(out) >= replyLimit {
			return
		}
		in.Actor = g.Actor
		out = append(out, Affordance{Intent: in, Check: checkOf(in.Verb), Reply: true})
	}

	// Расспросить про тему. Какую — см. weakestTopic. Тем известно ноль —
	// открытый вопрос: человек расскажет то, что готов рассказать.
	topic, hasTopic := g.weakestTopic()
	add(Intent{Verb: "question", Args: Args{Target: with, Topic: topic}}, hasTopic)
	add(Intent{Verb: "question", Args: Args{Target: with}}, !hasTopic)

	// Предъявить — один раз на человека. Показать ту же бумагу второй раз не
	// ход, а повтор: сдвиг расположения ядро всё равно даёт однократно.
	item, hasItem := g.unshownItem(with)
	add(Intent{Verb: "present", Args: Args{Item: item, Target: with}}, hasItem)

	// Спросить о мире. Законен всегда, о деле не выдаёт ничего — это та
	// реплика, которой держат разговор, когда по делу спросить нечего.
	add(Intent{Verb: "ask_about", Args: Args{Target: with}}, true)

	// Надавить — если первые три не набрали трёх.
	add(Intent{Verb: "threaten_verbally", Args: Args{Target: with}}, true)

	if exit, ok := g.exit(); ok {
		out = append(out, exit)
	}
	return out
}

// exit — вариант, которым выходят из разговора.
//
// Осмотр вперёд перехода: он оставляет игрока на месте, а выход из разговора
// не обязан быть уходом из сцены.
func (g *Game) exit() (Affordance, bool) {
	if prop, ok := g.firstProp(); ok {
		return Affordance{Intent: Intent{Verb: "examine", Actor: g.Actor,
			Args: Args{Target: prop}}, Check: checkOf("examine")}, true
	}
	if node, ok := g.firstKnownPlace(); ok {
		return Affordance{Intent: Intent{Verb: "move_zone", Actor: g.Actor,
			Args: Args{Node: node}}, Check: checkOf("move_zone")}, true
	}
	return Affordance{}, false
}

// weakestTopic — известный факт с наименьшей подтверждённостью.
//
// Правило не произвольное: детектив, обходящий свидетелей ради второго
// источника шаткой улики, занят ровно этим. Считается целиком из
// party_knowledge, без графа держателей, и вращается само — подтвердил,
// слабейшим стал другой. Ничьи решает порядок банка, то есть идентификатор.
func (g *Game) weakestTopic() (store.FactID, bool) {
	bank := g.K.TopicBank()
	if len(bank) == 0 {
		return "", false
	}
	weakest := bank[0]
	for _, f := range bank[1:] {
		if g.K.Confidence(f) < g.K.Confidence(weakest) {
			weakest = f
		}
	}
	return weakest, true
}

// unshownItem — первое носимое, которого этому человеку ещё не показывали.
func (g *Game) unshownItem(to store.EntityID) (string, bool) {
	for _, item := range g.Carried() {
		if !g.D.Presented(to, item.ID) {
			return string(item.ID), true
		}
	}
	return "", false
}
```

Прежнее тело `Affordances` целиком переименовать в `actions()`:

```go
// actions — что можно сделать, когда разговор не идёт.
func (g *Game) actions() []Affordance {
	var out []Affordance
	// ... прежнее тело без изменений ...
}
```

- [ ] **Шаг 4: Починить всех вызывающих**

Смена сигнатуры ломает существующие вызовы. Найти их все:

Запустить: `grep -rn "Affordances()" --include='*.go' .`

Ожидается четыре места: `cli/repl.go`, `cli/affordance_test.go`,
`core/affordance_test.go`, `tui/options_test.go` (через `Offered`).

`cli/repl.go`, в `offerAffordances`:

```go
	s.offered = s.Game.Affordances(s.spokenTo)
```

В тестах `core/affordance_test.go` и `cli/affordance_test.go` заменить
`g.Affordances()` на `g.Affordances("")` — они проверяют набор ВНЕ разговора,
и пустой собеседник это ровно он. `tui/options_test.go` правки не требует:
он ходит через сессию.

- [ ] **Шаг 5: Убедиться, что тесты проходят**

Запустить: `go test ./core/ ./cli/`
Ожидается: PASS.

- [ ] **Шаг 6: Прогон и коммит**

```bash
gofmt -l . && go test ./... && git add -A && git commit -m "$(cat <<'EOF'
feat(core): в разговоре набор — реплики, а не действия

Игрок заговорил со стражником и получил список из осмотра бочек и
перехода в кузницу. Разговор — половина детектива, и в наборе его не было.

Три реплики и один выход. Выход стоит последним и репликами не
вытесняется: из разговора должен быть выход одним нажатием, а не только
словами; осмотр вперёд перехода, потому что он оставляет игрока на месте.

Тема реплики — факт с наименьшей подтверждённостью. Правило не
произвольное: детектив, обходящий свидетелей ради второго источника
шаткой улики, занят ровно этим — и вращается оно само.

Вариант с темой появляется независимо от того, держит ли собеседник факт.
До снятия отказа в ядре это было невозможно: присутствие варианта было бы
ответом на вопрос, кто что знает.

Собеседник передаётся параметром, а не выводится изнутри ядра: память
разговора пишет актёр, и без моделей она пуста — набор реплик не
появлялся бы в прогоне без моделей вовсе.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Задача 3: Роль озвучки в реестре и проводке

**Файлы:**
- Правка: `llm/role.go` (константа и `AllRoles`)
- Правка: `llm/capability.go` (запись в `Capabilities`)
- Правка: `cmd/dnd/main.go` (`appRoles`, `buildRouter`)
- Тест: `llm/capability_test.go`

**Интерфейсы:**
- Производит: `llm.RoleOptions Role = "options"` с
  `Capability{Reads: {ReadScenePublic, ReadPartyKnowledge, ReadCarriedItems}}`
  и пустым `Proposes`.

- [ ] **Шаг 1: Написать падающий тест**

В `llm/capability_test.go`:

```go
// Роль озвучки вариантов ничего не предлагает ядру, и из этого выводится, что
// строгий провайдер ей не нужен: её выход не доходит до состояния, и плохой
// разбор стоит бледной строки, а не канона.
func TestOptionsRoleProposesNothing(t *testing.T) {
	cap, ok := Capabilities[RoleOptions]
	if !ok {
		t.Fatal("у роли озвучки вариантов нет капабилити")
	}
	if len(cap.Proposes) != 0 {
		t.Errorf("роль озвучки что-то предлагает ядру: %v", cap.Proposes)
	}
	if RequiresStrictOutput(RoleOptions) {
		t.Error("роли озвучки навязан строгий провайдер")
	}
}

// Скоуп ровно тот, из чего строится набор: сцена, знание парти, носимое.
// Правды дела в наборе ReadScope нет и быть не может по конструкции.
func TestOptionsRoleReadsOnlyWhatTheSetIsBuiltFrom(t *testing.T) {
	want := map[ReadScope]bool{
		ReadScenePublic: true, ReadPartyKnowledge: true, ReadCarriedItems: true,
	}
	for _, r := range Capabilities[RoleOptions].Reads {
		if !want[r] {
			t.Errorf("роль озвучки читает лишнее: %q", r)
		}
		delete(want, r)
	}
	for r := range want {
		t.Errorf("роль озвучки не читает %q — набор без этого не назвать словами", r)
	}
}
```

- [ ] **Шаг 2: Убедиться, что тест падает**

Запустить: `go test ./llm/ -run OptionsRole`
Ожидается: FAIL с `undefined: RoleOptions`.

- [ ] **Шаг 3: Реализация**

`llm/role.go` — константа рядом с остальными:

```go
	// RoleOptions — слова для набора вариантов. Роль своя, потому что роутинг
	// и тиринг идут по роли: назвать четыре строки словами игрока стоит иначе,
	// чем описать сцену, и выбирать модель для этого надо отдельно.
	RoleOptions Role = "options"
```

и в `AllRoles()` — после `RoleNarrator`:

```go
		RoleOptions,
```

`llm/capability.go` — перед `RoleWorldsmith`:

```go
	// RoleOptions даёт словам набора вариантов форму реплики. Предлагает
	// ПУСТО, и это несущее: её выход не доходит до состояния, поэтому строгий
	// провайдер ей не нужен, а плохой разбор стоит бледной строки, а не канона.
	//
	// Скоуп ровно тот, из чего набор и построен. Шире не нужно: назвать словами
	// можно только то, что уже выбрано кодом.
	RoleOptions: {
		Reads: []ReadScope{ReadScenePublic, ReadPartyKnowledge, ReadCarriedItems},
	},
```

`cmd/dnd/main.go` — в `appRoles`, после строки нарратора:

```go
	// Дешёвый тир и только он: формулировка четырёх строк — не та работа, за
	// которую платят основной моделью.
	{llm.RoleOptions, llm.TierCheap},
```

в `buildRouter` — в цепочку `Route`:

```go
		Route(llm.RoleOptions, target).
```

и в блок дешёвого тира:

```go
		router.RouteCheap(llm.RoleActor, cheap).
			RouteCheap(llm.RoleNarrator, cheap).
			RouteCheap(llm.RoleOptions, cheap)
```

- [ ] **Шаг 4: Убедиться, что тесты проходят**

Запустить: `go test ./llm/ ./cmd/dnd/ ./e2e/ -run 'OptionsRole|Role|Capabilit|Report'`
Ожидается: PASS. `TestEveryRoleHasCapability` и `TestReportCoversEveryRoutedRole`
подхватывают новую роль сами.

- [ ] **Шаг 5: Прогон и коммит**

```bash
gofmt -l . && go test ./... && git add -A && git commit -m "$(cat <<'EOF'
feat(llm): роль озвучки вариантов — дешёвый тир и пустой Proposes

Пустой Proposes здесь несущий, а не формальный: из него
RequiresStrictOutput выводит, что строгий провайдер роли не нужен. Её
выход не доходит до состояния, и плохой разбор стоит бледной строки, а не
канона.

Скоуп ровно тот, из чего набор и построен: сцена, знание парти, носимое.
Шире не нужно — назвать словами можно только то, что уже выбрано кодом.

Дешёвый тир и только он: формулировка четырёх строк не та работа, за
которую платят основной моделью.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Задача 4: Мастер озвучивает набор

**Файлы:**
- Создать: `master/options.go`
- Тест: `master/options_test.go`

**Интерфейсы:**
- Производит:
  ```go
  type Option struct {
      Text  string // кодовые слова варианта
      Reply bool   // это реплика, а не действие
  }
  func (m *Master) Options(ctx context.Context, w World, opts []Option,
      req llm.Request) ([]string, error)
  ```
  Возвращает ровно `len(opts)` строк либо ошибку. Частичного результата нет.

- [ ] **Шаг 1: Написать падающий тест**

`master/options_test.go`:

```go
package master

import (
	"context"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
)

func TestOptionsAreVoicedInOrder(t *testing.T) {
	m, f := masterWith(t, "Где вы были в ту ночь?\nВзгляните — предписание.\nосмотреть бочки")
	got, err := m.Options(context.Background(), World{Setting: "Гавань"}, []Option{
		{Text: "расспросить Берн про след шнура", Reply: true},
		{Text: "предъявить Предписание магистрата — Берн", Reply: true},
		{Text: "осмотреть Штабель бочек", Reply: false},
	}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("строк %d, вариантов было 3: %v", len(got), got)
	}
	if !strings.Contains(got[0], "ту ночь") {
		t.Errorf("первая строка не о первом варианте: %q", got[0])
	}
	if f.Calls()[0].Role != llm.RoleOptions {
		t.Errorf("озвучка вызвана чужой ролью: %q", f.Calls()[0].Role)
	}
	if f.Calls()[0].Tier != llm.TierCheap {
		t.Errorf("озвучка ушла тиром %q — это работа дешёвой модели", f.Calls()[0].Tier)
	}
}

// Слова применяются целиком либо не применяются вовсе. Частичное применение —
// худший из отказов: строка съезжает на соседний интент, и игрок, выбравший
// «поблагодарить», угрожает.
func TestPartialVoicingIsRefused(t *testing.T) {
	m, _ := masterWith(t, "Где вы были в ту ночь?\nВзгляните — предписание.")
	_, err := m.Options(context.Background(), World{}, []Option{
		{Text: "первый", Reply: true},
		{Text: "второй", Reply: true},
		{Text: "третий", Reply: false},
	}, llm.Request{})
	if err == nil {
		t.Fatal("две строки на три варианта приняты — строка съедет на соседний интент")
	}
}

// Пустой набор модель не беспокоит: вызов без нужды это деньги за шум.
func TestEmptySetIsNotSentToTheModel(t *testing.T) {
	m, f := masterWith(t, "что угодно")
	got, err := m.Options(context.Background(), World{}, nil, llm.Request{})
	if err != nil || got != nil {
		t.Errorf("пустой набор дал %v, %v", got, err)
	}
	if len(f.Calls()) != 0 {
		t.Error("на пустой набор потрачен вызов модели")
	}
}

// Мастеру видно, где реплика, а где действие: реплику он пишет от первого лица
// словами игрока, действие остаётся действием.
func TestPromptSeparatesRepliesFromActions(t *testing.T) {
	m, f := masterWith(t, "а\nб")
	m.Options(context.Background(), World{}, []Option{
		{Text: "расспросить Берн", Reply: true},
		{Text: "осмотреть бочки", Reply: false},
	}, llm.Request{})
	in := f.Calls()[0].Input
	if !strings.Contains(in, "РЕПЛИКА") || !strings.Contains(in, "ДЕЙСТВИЕ") {
		t.Errorf("Мастер не различает реплику и действие:\n%s", in)
	}
}
```

- [ ] **Шаг 2: Убедиться, что тест падает**

Запустить: `go test ./master/ -run 'Options|PartialVoicing|EmptySet|PromptSeparates'`
Ожидается: FAIL с `m.Options undefined`.

- [ ] **Шаг 3: Реализация**

`master/options.go`:

```go
package master

import (
	"context"
	"fmt"
	"strings"

	"github.com/kliuchnikovv/dnd/llm"
)

// Слова для набора вариантов.
//
// Набор выбирает код — детерминированно и из read-scope. Мастер только даёт
// выбранному слова, и вернуть он обязан ровно столько строк, сколько получил
// вариантов: строка, съехавшая на соседний интент, — худший из возможных
// отказов, потому что игрок, выбравший «поблагодарить», угрожает.

// Option — вариант, которому нужны слова.
type Option struct {
	// Text — кодовые слова варианта: то, что напечатается, если озвучки нет.
	Text string
	// Reply — это реплика, а не действие. Реплику Мастер пишет от первого лица
	// словами игрока; действие остаётся действием.
	Reply bool
}

const optionsSystem = `Ты Мастер настольной игры. Тебе дают список вариантов хода,
уже выбранных игрой, и ты называешь каждый словами.

Правила, которые нельзя нарушать:
1. Строк в ответе РОВНО столько, сколько вариантов, и в том же порядке. Одна
   строка на вариант, без нумерации и без пустых строк.
2. Помеченное РЕПЛИКА — это то, что игрок СКАЖЕТ вслух. Пиши от первого лица,
   живой речью, одной фразой. Без кавычек — их поставит игра.
3. Помеченное ДЕЙСТВИЕ — это то, что игрок СДЕЛАЕТ. Короткая фраза в
   неопределённой форме: «осмотреть штабель бочек».
4. Ничего не добавляй от себя: ни исхода, ни догадки о деле, ни намёка на то,
   где искать. Ты называешь уже выбранное, а не советуешь.
5. Языком мира, на «вы». Ни чисел, ни механики, ни служебных пометок.`

// Options даёт словам набора форму реплики. Возвращает ровно столько строк,
// сколько получил вариантов, либо ошибку: частичного результата не бывает.
//
// Пустой набор модель не беспокоит: вызов без нужды это деньги за шум.
func (m *Master) Options(ctx context.Context, w World, opts []Option,
	req llm.Request) ([]string, error) {
	if len(opts) == 0 {
		return nil, nil
	}

	req.Role = llm.RoleOptions
	req.Tier = llm.TierCheap
	req.Schema = ""
	req.System = optionsSystem
	if req.MaxTokens == 0 {
		// Четыре короткие строки плюс запас на рассуждение.
		req.MaxTokens = 400
	}

	var b strings.Builder
	b.WriteString("Варианты, которым нужны слова:\n")
	for _, o := range opts {
		kind := "ДЕЙСТВИЕ"
		if o.Reply {
			kind = "РЕПЛИКА"
		}
		b.WriteString("  " + kind + ": " + o.Text + "\n")
	}
	w.render(&b)
	b.WriteString(fmt.Sprintf("\nВерни РОВНО %d строк, по одной на вариант, в том же порядке.\n",
		len(opts)))
	req.Input = b.String()

	resp, err := m.gw.Do(ctx, req)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, line := range strings.Split(resp.Text, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			lines = append(lines, s)
		}
	}
	if len(lines) != len(opts) {
		// Всё или ничего. Приложить что пришло к первым вариантам значило бы
		// подписать одну строку под другой интент, и выбор игрока сделал бы
		// не то, что он прочёл.
		return nil, fmt.Errorf("master: строк %d на %d вариантов", len(lines), len(opts))
	}
	return lines, nil
}
```

- [ ] **Шаг 4: Убедиться, что тесты проходят**

Запустить: `go test ./master/ -run 'Options|PartialVoicing|EmptySet|PromptSeparates' -v`
Ожидается: PASS все четыре.

- [ ] **Шаг 5: Прогон и коммит**

```bash
gofmt -l . && go test ./... && git add -A && git commit -m "$(cat <<'EOF'
feat(master): слова для набора вариантов — всё или ничего

Набор выбирает код, Мастер даёт выбранному слова. Вернуть он обязан ровно
столько строк, сколько получил вариантов: строка, съехавшая на соседний
интент, — худший из возможных отказов, потому что игрок, выбравший
«поблагодарить», угрожает. Несовпадение числа это ошибка, а не повод
приложить что пришло к первым вариантам.

Пустой набор модель не беспокоит: вызов без нужды это деньги за шум.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Задача 5: Озвучка в сессии — подключение, кэш, откат

**Файлы:**
- Правка: `cli/affordance.go` (интерфейс, кэш, сборка вариантов)
- Правка: `cli/repl.go` (`offerAffordances`, поле воизера, `OfferedWords`)
- Правка: `cli/render.go` (`Affordances` принимает слова)
- Правка: `cmd/dnd/main.go` (адаптер и проводка)
- Правка: `tui/options.go` (панель берёт слова)
- Тест: `cli/affordance_test.go`

**Интерфейсы:**
- Потребляет: `master.Options` из задачи 4.
- Производит:
  ```go
  type Option struct { Text string; Reply bool }
  type OptionVoicer interface {
      VoiceOptions(ctx context.Context, opts []Option) ([]string, error)
  }
  func (s *Session) WithOptionVoicer(v OptionVoicer) *Session
  func (s *Session) OfferedWords() []string
  func (r Render) Affordances(g *core.Game, list []core.Affordance, words []string) string
  ```

- [ ] **Шаг 1: Написать падающий тест**

В `cli/affordance_test.go`:

```go
type fakeVoicer struct {
	lines []string
	err   error
	calls int
	seen  [][]Option
}

func (f *fakeVoicer) VoiceOptions(_ context.Context, opts []Option) ([]string, error) {
	f.calls++
	f.seen = append(f.seen, opts)
	return f.lines, f.err
}

// Слова Мастера заменяют кодовые. Без них список читается как перечень команд,
// а не как разговор.
func TestVoicedWordsReplaceTheCodeOnes(t *testing.T) {
	g := renderGame(t)
	v := &fakeVoicer{lines: []string{"раз", "два", "три", "четыре"}}
	var out strings.Builder
	s := NewSession(g, strings.NewReader("quit\n"), &out).
		WithAffordances().WithOptionVoicer(v)
	s.Start()
	if got := out.String(); !strings.Contains(got, "раз") {
		t.Errorf("слова Мастера не напечатаны:\n%s", got)
	}
}

// Число не совпало — печатаются кодовые слова, все до одной. Приложить что
// пришло к первым вариантам значило бы подписать строку под чужой интент.
func TestMismatchedVoicingFallsBackWholesale(t *testing.T) {
	g := renderGame(t)
	v := &fakeVoicer{lines: []string{"раз", "два"}}
	var out strings.Builder
	s := NewSession(g, strings.NewReader("quit\n"), &out).
		WithAffordances().WithOptionVoicer(v)
	s.Start()
	got := out.String()
	if strings.Contains(got, "раз") {
		t.Errorf("частичная озвучка применена:\n%s", got)
	}
	if !strings.Contains(got, "заговорить") {
		t.Errorf("откат на кодовые слова не сработал:\n%s", got)
	}
}

// Сбой озвучки ход не рушит: надстройка не должна быть условием работы.
func TestVoicingFailureKeepsTheGameRunning(t *testing.T) {
	g := renderGame(t)
	v := &fakeVoicer{err: errors.New("шлюз закрыт")}
	var out strings.Builder
	s := NewSession(g, strings.NewReader("quit\n"), &out).
		WithAffordances().WithOptionVoicer(v)
	s.Start()
	if !strings.Contains(out.String(), "заговорить") {
		t.Errorf("сбой озвучки съел список:\n%s", out.String())
	}
}

// Набор часто повторяется от хода к ходу, и повтор платить не должен.
func TestUnchangedSetIsNotVoicedTwice(t *testing.T) {
	g := renderGame(t)
	v := &fakeVoicer{lines: []string{"раз", "два", "три", "четыре"}}
	s := NewSession(g, strings.NewReader("survey\nsurvey\nquit\n"), &strings.Builder{}).
		WithAffordances().WithOptionVoicer(v)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if v.calls != 1 {
		t.Errorf("вызовов озвучки %d на неизменившийся набор", v.calls)
	}
}

// Мастеру видно, где реплика: без этого он напишет действие от первого лица
// или реплику в неопределённой форме.
func TestVoicerIsToldWhichOptionsAreReplies(t *testing.T) {
	g := renderGame(t)
	v := &fakeVoicer{lines: []string{"раз", "два", "три", "четыре"}}
	s := NewSession(g, strings.NewReader("quit\n"), &strings.Builder{}).
		WithAffordances().WithOptionVoicer(v)
	s.Start()
	if len(v.seen) == 0 {
		t.Fatal("озвучка не вызвана")
	}
	for i, o := range v.seen[0] {
		if o.Text == "" {
			t.Errorf("вариант %d ушёл на озвучку без кодовых слов", i)
		}
	}
}
```

- [ ] **Шаг 2: Убедиться, что тест падает**

Запустить: `go test ./cli/ -run 'Voiced|Mismatched|VoicingFailure|UnchangedSet|VoicerIsTold'`
Ожидается: FAIL с `WithOptionVoicer undefined`.

- [ ] **Шаг 3: Реализация в `cli/affordance.go`**

```go
// Option — вариант, которому нужны слова. Копия формы, а не структура master:
// cli не импортирует надстройки, иначе направление слоёв развернулось бы.
type Option struct {
	Text  string
	Reply bool
}

// OptionVoicer — необязательные слова для набора. Без него печатаются кодовые:
// игра без моделей обязана работать как работала.
type OptionVoicer interface {
	VoiceOptions(ctx context.Context, opts []Option) ([]string, error)
}

// optionsFor — набор в форме заказа на слова.
func optionsFor(g *core.Game, list []core.Affordance) []Option {
	out := make([]Option, 0, len(list))
	for _, a := range list {
		out = append(out, Option{Text: AffordanceLabel(g, a), Reply: a.Reply})
	}
	return out
}

// optionsKey — отпечаток набора. Слова просятся только когда набор сменился:
// он часто повторяется от хода к ходу, и повтор платить не должен.
func optionsKey(list []core.Affordance) string {
	var b strings.Builder
	for _, a := range list {
		fmt.Fprintf(&b, "%s|%s|%s|%s|%s;", a.Intent.Verb, a.Intent.Args.Target,
			a.Intent.Args.Topic, a.Intent.Args.Node, a.Intent.Args.Item)
	}
	return b.String()
}
```

Правка `AffordanceLabel` — реплики в кавычках делает задача 6; сейчас без изменений.

- [ ] **Шаг 4: Реализация в `cli/repl.go`**

Поля рядом с `offered`:

```go
	// optionVoicer — необязательные слова Мастера для набора.
	optionVoicer OptionVoicer
	// offeredWords — слова показанного набора; пусто означает кодовые.
	// voicedKey — отпечаток набора, для которого слова уже спрошены.
	offeredWords []string
	voicedKey    string
```

Включатель рядом с `WithAffordances`:

```go
// WithOptionVoicer отдаёт слова набора Мастеру. Сбой и потолок расхода
// откатывают на кодовые слова: список без слов хуже кодового списка, а
// список без модели обязан работать как работал.
func (s *Session) WithOptionVoicer(v OptionVoicer) *Session {
	s.optionVoicer = v
	return s
}

// OfferedWords — слова показанного набора. Пусто означает, что печатаются
// кодовые: полноэкранная панель обязана видеть ровно то, что напечатал
// построчный режим.
func (s *Session) OfferedWords() []string { return s.offeredWords }
```

`offerAffordances` целиком:

```go
func (s *Session) offerAffordances() {
	if !s.affordances {
		return
	}
	s.offered = s.Game.Affordances(s.spokenTo)
	s.offeredWords = s.voiceOptions(s.offered)
	if text := s.r.Affordances(s.Game, s.offered, s.offeredWords); text != "" {
		s.emitText(EventOptions, text)
	}
}

// voiceOptions просит слова у Мастера. Пустой ответ означает кодовые слова —
// и это законный исход: сбой надстройки не рушит ход.
//
// Слова применяются целиком либо не применяются вовсе. Частичное применение
// подписало бы строку под соседний интент, и игрок, выбравший «поблагодарить»,
// угрожал бы.
func (s *Session) voiceOptions(list []core.Affordance) []string {
	if s.optionVoicer == nil || len(list) == 0 {
		return nil
	}
	key := optionsKey(list)
	if key == s.voicedKey {
		return s.offeredWords
	}
	s.voicedKey = key
	words, err := s.optionVoicer.VoiceOptions(s.turnContext(), optionsFor(s.Game, list))
	if err != nil {
		s.noteOnce("Мастер не назвал варианты: " + err.Error())
		return nil
	}
	if len(words) != len(list) {
		return nil
	}
	// Слова — вывод модели по недоверенному вводу, и отвечают они за себя
	// отдельно от разбора: своя строка аудита при той же команде.
	s.noteProposal(llm.RoleOptions, llmProposal{Options: words})
	return words
}
```

Поле аудита в `cli/journal.go`, рядом с `Probe`:

```go
	// Options — слова, которыми Мастер назвал набор вариантов. В журнал команд
	// они не идут: правда реплея — интент, а слова живут ровно один показ.
	Options []string `json:"options,omitempty"`
```

- [ ] **Шаг 5: Реализация в `cli/render.go`**

```go
// Affordances печатает набор вариантов нумерованным списком.
//
// words — слова Мастера; пусто либо не той длины означает кодовые. Длина
// сверяется и здесь, хотя её уже сверила сессия: печать — последнее место,
// где строка может съехать на соседний интент, и второй раз это дешевле, чем
// один раз не проверить.
func (r Render) Affordances(g *core.Game, list []core.Affordance, words []string) string {
	if len(list) == 0 {
		return ""
	}
	if len(words) != len(list) {
		words = nil
	}
	var b strings.Builder
	b.WriteString("Что можно:\n")
	for i, a := range list {
		label := AffordanceLabel(g, a)
		if words != nil {
			label = words[i]
		}
		fmt.Fprintf(&b, "  %d. %s\n", i+1, label)
	}
	b.WriteString(affordancePrompt)
	return b.String()
}
```

- [ ] **Шаг 6: Проводка в `cmd/dnd/main.go`**

К типу `narrator` добавить метод (голос один, потому что говорящий один):

```go
// VoiceOptions называет варианты словами. Тот же Мастер, что ведёт прозу:
// делить список и сцену между двумя голосами значило бы говорить с игроком
// двумя разными людьми.
func (n *narrator) VoiceOptions(ctx context.Context, opts []cli.Option) ([]string, error) {
	out := make([]master.Option, 0, len(opts))
	for _, o := range opts {
		out = append(out, master.Option{Text: o.Text, Reply: o.Reply})
	}
	return n.master.Options(ctx,
		master.World{Setting: n.game.Setting, Scene: sceneFor(n.game)}, out, llm.Request{})
}
```

и в проводке рядом с `WithNarrator`:

```go
		session.WithNarrator(voice).WithRefuser(voice).WithOptionVoicer(voice)
```

- [ ] **Шаг 7: Панель полноэкранного режима берёт слова**

`tui/options.go`, в `optionsView` заменить сборку строки:

```go
	words := m.session.OfferedWords()
	for i, a := range m.offered {
		label := cli.AffordanceLabel(m.session.Game, a)
		if len(words) == len(m.offered) {
			label = words[i]
		}
		line := "  " + strconv.Itoa(i+1) + ". " + label
		...
	}
```

и в `chosenEcho` — тем же способом, чтобы в транскрипт попало то, что игрок
прочёл, а не кодовые слова.

- [ ] **Шаг 8: Убедиться, что тесты проходят**

Запустить: `go test ./cli/ ./tui/ ./cmd/dnd/`
Ожидается: PASS.

- [ ] **Шаг 9: Прогон и коммит**

```bash
gofmt -l . && go test ./... && git add -A && git commit -m "$(cat <<'EOF'
feat(cli): слова набора от Мастера, кодовые как дно

Список читался перечнем команд, а не разговором. Теперь слова даёт
Мастер — но набор по-прежнему выбирает код, и это не косметика: слова не
доходят до Feed вовсе, выбор уезжает номером, и правда реплея не меняется.

Всё или ничего, и проверяется дважды — в сессии и на печати. Печать
последнее место, где строка может съехать на соседний интент, и второй
раз проверить дешевле, чем один раз не проверить.

Слова просятся, только когда набор сменился: он часто повторяется от хода
к ходу, и повтор платить не должен.

Сбой, потолок расхода, выключенные модели — кодовые слова и жалоба один
раз на прогон. Молча откатываясь, игра выглядит рабочей при выключенном
Мастере.

Формулировка ложится в аудит и не ложится в журнал команд: слова живут
ровно один показ.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Задача 6: Кавычки у реплик

**Файлы:**
- Правка: `cli/affordance.go` (`AffordanceLabel`, `Render.Affordances`)
- Правка: `cli/affordance_test.go` (сузить тест на речь)

**Интерфейсы:**
- Потребляет: `core.Affordance.Reply` из задачи 2, `words` из задачи 5.

- [ ] **Шаг 1: Написать падающий тест**

```go
// Реплики печатаются в кавычках, действия — нет: игрок должен видеть, где он
// говорит, а где делает.
func TestRepliesAreQuotedAndActionsAreNot(t *testing.T) {
	g := renderGame(t)
	list := []core.Affordance{
		{Intent: core.Intent{Verb: "ask_about", Args: core.Args{Target: "e_toke"}}, Reply: true},
		{Intent: core.Intent{Verb: "examine", Args: core.Args{Target: "p_crates"}}},
	}
	out := Render{}.Affordances(g, list, []string{"Как тут живут?", "осмотреть ящики"})
	if !strings.Contains(out, "«Как тут живут?»") {
		t.Errorf("реплика без кавычек:\n%s", out)
	}
	if strings.Contains(out, "«осмотреть ящики»") {
		t.Errorf("действие взято в кавычки:\n%s", out)
	}
}

// Кавычки ставит презентация, а не Мастер: две пары кавычек подряд — это
// сломанная строка, и увидит её игрок, а не тест Мастера.
func TestQuotesAreNotDoubled(t *testing.T) {
	g := renderGame(t)
	list := []core.Affordance{
		{Intent: core.Intent{Verb: "ask_about", Args: core.Args{Target: "e_toke"}}, Reply: true},
	}
	out := Render{}.Affordances(g, list, []string{"«Как тут живут?»"})
	if strings.Contains(out, "««") {
		t.Errorf("кавычки удвоены:\n%s", out)
	}
}
```

- [ ] **Шаг 2: Убедиться, что тест падает**

Запустить: `go test ./cli/ -run 'RepliesAreQuoted|QuotesAreNotDoubled'`
Ожидается: FAIL — кавычек нет.

- [ ] **Шаг 3: Реализация**

В `cli/affordance.go` — в цикле печати `Render.Affordances`, после выбора
`label`:

```go
		if a.Reply {
			label = quoted(label)
		}
```

и рядом:

```go
// quoted берёт реплику в кавычки. Уже закавыченную не удваивает: Мастера
// просили писать без кавычек, но просьба — не гарантия, а две пары подряд
// увидит игрок, а не тест.
func quoted(line string) string {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "«") && strings.HasSuffix(line, "»") {
		return line
	}
	return "«" + line + "»"
}
```

- [ ] **Шаг 4: Сузить тест на речь**

`TestAffordanceLinesAreNotMistakenForSpeech` проверял, что ни одна строка
списка не разбирается как речь. У реплик в кавычках это перестало быть верным —
и это правильно. Заменить тело на проверку строк-действий и добавить пояснение:

```go
// Ни одна строка-ДЕЙСТВИЕ не начинается с тире и не берётся в кавычки: такая
// строка разбирается как прямая речь, и перепечатанный игроком вариант ушёл бы
// в say вместо осмотра.
//
// К репликам это не относится намеренно. Для реплики слова И ЕСТЬ то, что
// игрок сказал бы, и фраза, ушедшая в say, — связный исход, а не поломка.
func TestActionLinesAreNotMistakenForSpeech(t *testing.T) {
	g := renderGame(t)
	list := g.Affordances("")
	for i, line := range strings.Split(Render{}.Affordances(g, list, nil), "\n") {
		if _, _, ok := Speech(line); ok {
			t.Errorf("строка %d разбирается как речь: %q", i, line)
		}
	}
}
```

- [ ] **Шаг 5: Убедиться, что тесты проходят**

Запустить: `go test ./cli/`
Ожидается: PASS.

- [ ] **Шаг 6: Прогон и коммит**

```bash
gofmt -l . && go test ./... && git add -A && git commit -m "$(cat <<'EOF'
feat(cli): реплики в кавычках, действия — нет

Игрок должен видеть, где он говорит, а где делает: список, в котором
«осмотреть бочки» и «а где вы были в ту ночь?» выглядят одинаково, читается
как перечень команд.

Кавычки ставит презентация, а не Мастер: его просили писать без них, но
просьба не гарантия, и две пары подряд увидит игрок, а не тест.

Тест на речь сузился до строк-действий. У реплик в кавычках прежнее
рассуждение перестало быть верным — и это правильно: для реплики слова и
есть то, что игрок сказал бы, а перепечатанная фраза, ушедшая в say, —
связный исход, а не поломка.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Задача 7: Инварианты набора на данных дел

**Файлы:**
- Правка: `e2e/affordance_test.go`

**Интерфейсы:**
- Потребляет: `Game.Affordances(store.EntityID)` из задачи 2.

- [ ] **Шаг 1: Написать падающий тест**

```go
// Меню не врёт: ни один вариант, включая реплики, ядро не отклоняет.
//
// Оговорка, без которой тест был бы ложно-зелёным: Check покрывает не все
// отказы. Отказы предъявления живут в Apply — present проверяет предмет и
// адресата уже внутри хода. Исполнить ход здесь нельзя, он сбил бы прогон,
// поэтому его условия проверяются напрямую.
func TestNoAffordanceIsRefused(t *testing.T) {
	for _, path := range affordanceCases {
		g := affordanceGame(t, path)
		r := rand.New(rand.NewSource(3))
		for turn := 0; turn < 100; turn++ {
			for _, npc := range append(npcsAt(g), "") {
				for _, a := range g.Affordances(npc) {
					if res := g.Check(a.Intent); res.Refused {
						t.Fatalf("%s: вариант %s → %q отклонён: %s",
							path, a.Intent.Verb, a.Intent.Args.Target, res.Refusal)
					}
					if a.Intent.Verb != "present" {
						continue
					}
					if !g.Carries(store.ItemID(a.Intent.Args.Item)) {
						t.Fatalf("%s: предложено предъявить ненесомое %q",
							path, a.Intent.Args.Item)
					}
					if a.Intent.Args.Target == "" {
						t.Fatalf("%s: предъявление предложено без адресата", path)
					}
				}
			}
			playRandomly(g, r, 1)
		}
	}
}

// Реплика называет только то, что игрок и так видит либо знает.
func TestRepliesNameOnlyWhatThePlayerKnows(t *testing.T) {
	for _, path := range affordanceCases {
		g := affordanceGame(t, path)
		r := rand.New(rand.NewSource(5))
		for turn := 0; turn < 100; turn++ {
			visible := readScopeOf(g)
			for _, npc := range npcsAt(g) {
				for _, a := range g.Affordances(npc) {
					if !a.Reply {
						continue
					}
					for _, name := range []string{
						string(a.Intent.Args.Target), string(a.Intent.Args.Topic),
						a.Intent.Args.Item,
					} {
						if name != "" && !visible[name] {
							t.Fatalf("%s: реплика назвала %q, которого игрок не видит",
								path, name)
						}
					}
				}
			}
			playRandomly(g, r, 1)
		}
	}
}

func npcsAt(g *core.Game) []store.EntityID {
	var out []store.EntityID
	for _, e := range g.DB.EntitiesAt(g.Node) {
		if e.Kind == store.EntityNPC {
			out = append(out, e.ID)
		}
	}
	return out
}
```

- [ ] **Шаг 2: Убедиться, что тест падает или проходит осмысленно**

Запустить: `go test ./e2e/ -run 'NoAffordanceIsRefused|RepliesNameOnly' -v`
Ожидается: PASS, если задачи 1–2 сделаны верно. FAIL здесь означает реальную
дыру — читать сообщение, а не подгонять тест.

- [ ] **Шаг 3: Прогон и коммит**

```bash
gofmt -l . && go test ./... && git add -A && git commit -m "$(cat <<'EOF'
test(e2e): меню не врёт и не спойлерит — на данных обоих дел

Инвариант проверяется с оговоркой, без которой тест был бы ложно-зелёным:
Check покрывает не все отказы. Отказы предъявления живут в Apply — present
проверяет предмет и адресата уже внутри хода, — поэтому их условия
проверяются отдельно и напрямую.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Задача 8: Живая проверка (за пользователем)

```bash
go run ./cmd/dnd --case cases/harbour/case.json --chat \
  --provider openrouter --model anthropic/claude-sonnet-5 \
  --model-cheap anthropic/claude-haiku-4.5 \
  --price-in 2 --price-out 10 --price-in-cheap 1 --price-out-cheap 5 \
  --cap-day 0.5
```

Смотреть: заговорив с человеком, игрок видит реплики, а не осмотр бочек;
реплики читаются как речь и не спойлерят; расспрос незнающего даёт «не по делу»
вместо отказа; в отчёте появилась строка `вызовов options` с ценой дешёвого
тира.
