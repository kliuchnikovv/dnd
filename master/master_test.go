package master

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/guard"
	"github.com/kliuchnikovv/dnd/llm"
)

// stubChecker — проверка прозы без сети. verdicts задаёт вердикт по вызовам:
// ремонт проверяется вторым разом. Отказ трактуется как противоречие состоянию.
type stubChecker struct {
	verdicts     []bool
	what         string
	stateSeen    []string
	materialSeen []string
	calls        int
}

func (c *stubChecker) Check(_ context.Context, line string, material, state []string,
	_ llm.Request) (guard.Verdict, error) {
	c.materialSeen, c.stateSeen = material, state
	ok := true
	if len(c.verdicts) > 0 {
		ok = c.verdicts[min(c.calls, len(c.verdicts)-1)]
	}
	c.calls++
	v := guard.Verdict{OK: ok, What: c.what}
	if !ok {
		v.ContradictsState = true
	}
	return v, nil
}

func masterWith(t *testing.T, reply string) (*Master, *llm.Fake) {
	t.Helper()
	f := llm.NewFake("fake", true).ReplyWith(func(llm.Request) string { return reply })
	gw := llm.NewGateway(
		llm.NewRouter().
			Route(llm.RoleNarrator, llm.Target{Provider: f, Model: "claude-haiku-4-5"}).
			Route(llm.RoleOptions, llm.Target{Provider: f, Model: "claude-haiku-4-5"}),
		llm.NewLedger(llm.Caps{}))
	return New(gw), f
}

// Мастер — единственная власть над миром: он отвечает на то, чего у персонажа
// нет, и помечает решённое каноном.
func TestGrantAnswersNeeds(t *testing.T) {
	m, f := masterWith(t, `{"grants":[{"topic":"ключи от весовой",`+
		`"answer":"ключи у смотрителя весов, он же запирает на ночь","canon":true}]}`)

	grants, refused, err := m.Grant(context.Background(),
		[]string{"кто держит ключи от весовой"}, nil,
		World{Setting: "посёлок в устье", Scene: []string{"Место: Пристань"}}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if len(refused) != 0 {
		t.Errorf("отказ там, где был ответ: %v", refused)
	}
	if len(grants) != 1 || !grants[0].Canon {
		t.Fatalf("ответ Мастера не разобрался: %+v", grants)
	}
	if grants[0].Topic != "ключи от весовой" ||
		!strings.Contains(grants[0].Answer, "смотрителя весов") {
		t.Errorf("ответ не тот: %+v", grants[0])
	}
	if f.Calls()[0].Role != llm.RoleNarrator {
		t.Errorf("Мастер вызван не своей ролью: %q", f.Calls()[0].Role)
	}
}

// Отказ — законный исход: territory дела Мастеру не принадлежит.
func TestGrantRefusalIsALegalOutcome(t *testing.T) {
	m, _ := masterWith(t, `{"grants":[],"refuse":["кто убил Халдена"]}`)

	grants, refused, err := m.Grant(context.Background(),
		[]string{"кто убил Халдена"}, nil, World{}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 0 {
		t.Errorf("Мастер выдал содержание дела: %+v", grants)
	}
	if len(refused) != 1 || refused[0] != "кто убил Халдена" {
		t.Errorf("отказ не разобрался: %v", refused)
	}
}

// Пустой запрос модель не беспокоит: вызов без нужды это деньги за шум.
func TestGrantWithoutNeedsDoesNotCallTheModel(t *testing.T) {
	m, f := masterWith(t, `{"grants":[]}`)
	grants, refused, err := m.Grant(context.Background(), nil, nil, World{}, llm.Request{})
	if err != nil || len(grants) != 0 || len(refused) != 0 {
		t.Errorf("пустой запрос дал %+v %v %v", grants, refused, err)
	}
	if len(f.Calls()) != 0 {
		t.Error("модель вызвана впустую")
	}
}

// Уже решённое Мастер видит: иначе он решит то же второй раз и по-другому.
func TestGrantSeesExistingCanon(t *testing.T) {
	m, f := masterWith(t, `{"grants":[]}`)
	_, _, err := m.Grant(context.Background(), []string{"аптека"},
		[]CanonFact{{Topic: "аптека", Text: "аптеки нет, только травница"}}, World{}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if in := f.Calls()[0].Input; !strings.Contains(in, "травница") {
		t.Errorf("канон не доехал до Мастера:\n%s", in)
	}
}

// Решение ambient-детали — это «да/нет и одна строка». Дорогая модель ей не
// нужна: за неё платится в каждом разговоре.
func TestGrantGoesCheap(t *testing.T) {
	m, f := masterWith(t, `{"grants":[]}`)
	m.Grant(context.Background(), []string{"аптека"}, nil, World{}, llm.Request{})
	if got := f.Calls()[0].Tier; got != llm.TierCheap {
		t.Errorf("запрос к Мастеру пошёл тиром %q", got)
	}
}

// Нарратив: авторская рамка не заменяется, а оживляется. Противоречить ей
// Мастер не вправе, поэтому она обязана доехать.
func TestNarrateCarriesFrameSceneAndOutcome(t *testing.T) {
	m, f := masterWith(t, "Дождь не унимается, и доски под ногами скользят.")
	got, err := m.Narrate(context.Background(), KindOutcome,
		"Дождь сечёт доски пристани.", World{Scene: []string{"Место: Пристань"}},
		[]string{"успех, маржа +3", "узнали: тело найдено на складе"}, "", nil, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Дождь не унимается, и доски под ногами скользят." {
		t.Errorf("проза Мастера %q", got)
	}
	in := f.Calls()[0].Input
	for _, want := range []string{"Дождь сечёт доски", "Место: Пристань", "маржа +3"} {
		if !strings.Contains(in, want) {
			t.Errorf("в промпте нет %q:\n%s", want, in)
		}
	}
}

// Мастеру нельзя вводить содержание, похожее на дело: containment живёт на
// его границе, а не размазан по каждой реплике.
func TestPromptsForbidCaseContent(t *testing.T) {
	m, f := masterWith(t, `{"grants":[]}`)
	m.Grant(context.Background(), []string{"аптека"}, nil, World{}, llm.Request{})
	m.Narrate(context.Background(), KindPlace, "рамка", World{}, nil, "", nil, llm.Request{})

	for i, call := range f.Calls() {
		sys := strings.ToLower(call.System)
		if !strings.Contains(sys, "дел") || !strings.Contains(sys, "автор") {
			t.Errorf("вызов %d: в промпте нет границы дела:\n%s", i, call.System)
		}
	}
}

// Схема запроса — форма ответа Мастера. Без canon флага деталь не осядет в
// каноне, и второй вопрос даст новую выдумку.
func TestGrantSchemaCarriesCanonFlag(t *testing.T) {
	m, f := masterWith(t, `{"grants":[]}`)
	m.Grant(context.Background(), []string{"аптека"}, nil, World{}, llm.Request{})

	var probe map[string]any
	if err := json.Unmarshal([]byte(f.Calls()[0].Schema), &probe); err != nil {
		t.Fatal(err)
	}
	props := probe["properties"].(map[string]any)
	item := props["grants"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	for _, want := range []string{"topic", "answer", "canon"} {
		if _, ok := item[want]; !ok {
			t.Errorf("в схеме ответа нет поля %q", want)
		}
	}
	if _, ok := props["refuse"]; !ok {
		t.Error("в схеме нет отказа — а отказ законный исход")
	}
}

// Сцена и исход — разные задачи. Один промпт на оба давал «ты подходишь
// ближе» там, где игрок озирается по сторонам.
func TestNarrateTellsSceneFromOutcome(t *testing.T) {
	m, f := masterWith(t, "проза")
	m.Narrate(context.Background(), KindPlace, "рамка", World{}, nil, "", nil, llm.Request{})
	m.Narrate(context.Background(), KindOutcome, "рамка", World{},
		[]string{"исход: успех"}, "", nil, llm.Request{})

	if got := f.Calls()[0].Input; !strings.Contains(got, "ОПИСАНИЕ МЕСТА") {
		t.Errorf("сцена не помечена как описание места:\n%s", got)
	}
	if got := f.Calls()[1].Input; !strings.Contains(got, "ИСХОД") {
		t.Errorf("исход не помечен как исход:\n%s", got)
	}
}

// Проза исхода идёт ПЕРЕД репликой персонажа и успевает ему противоречить:
// «отвечает охотнее, чем можно было ждать» — а следом сухое «Что вам
// надобно?». За того, кто сейчас заговорит, Мастер не говорит.
func TestNarrateDoesNotSpeakForTheOneAboutToAnswer(t *testing.T) {
	m, f := masterWith(t, "проза")
	m.Narrate(context.Background(), KindOutcome, "рамка", World{},
		[]string{"исход: успех"}, "Берн, стражник", nil, llm.Request{})

	in := f.Calls()[0].Input
	if !strings.Contains(in, "СЕЙЧАС ОТВЕТИТ Берн, стражник") {
		t.Errorf("Мастеру не сказано, кто сейчас заговорит:\n%s", in)
	}
	if !strings.Contains(in, "Не говори за него") {
		t.Errorf("Мастеру не запрещено говорить за него:\n%s", in)
	}
}

// Там, где отвечать некому, запрета нет: лишняя строка в промпте это шум,
// за который платят каждый ход.
func TestNarrateWithoutSpeakerStaysClean(t *testing.T) {
	m, f := masterWith(t, "проза")
	m.Narrate(context.Background(), KindPlace, "рамка", World{}, nil, "", nil, llm.Request{})
	if in := f.Calls()[0].Input; strings.Contains(in, "СЕЙЧАС ОТВЕТИТ") {
		t.Errorf("запрет появился там, где никто не отвечает:\n%s", in)
	}
}

// KindReply озвучивает персонажа прямой речью: система — replySystem, тело
// велит говорить за него, а запрета «не говори за него» здесь нет (он для
// обрамляющего исхода, а не для самой реплики).
func TestReplyVoicesTheCharacter(t *testing.T) {
	m, f := masterWith(t, "— Не знаю никакого кассира.")
	m.Narrate(context.Background(), KindReply, "Берн уходит от ответа", World{},
		[]string{"исход: провал"}, "Берн", nil, llm.Request{})

	call := f.Calls()[0]
	if !strings.Contains(call.Input, "РЕПЛИКА") || !strings.Contains(call.Input, "ты Берн") {
		t.Errorf("reply-тело не велит говорить за персонажа:\n%s", call.Input)
	}
	if strings.Contains(call.Input, "Не говори за него") {
		t.Errorf("в реплике не должно быть запрета озвучивать персонажа:\n%s", call.Input)
	}
	if !strings.Contains(call.System, "прямой речью") {
		t.Errorf("система реплики не про прямую речь:\n%s", call.System)
	}
}

// KindClarify описывает неясное обращение диегетически: мир не понял игрока,
// без служебного «уточни» и без разрешения хода.
func TestClarifyNarratesDiegetically(t *testing.T) {
	m, f := masterWith(t, "Ваши слова тонут в шуме дождя.")
	m.Narrate(context.Background(), KindClarify, "слова повисли без ответа", World{},
		nil, "", nil, llm.Request{})
	in := f.Calls()[0].Input
	if !strings.Contains(in, "НЕЯСНОЕ ОБРАЩЕНИЕ") {
		t.Errorf("clarify-тело не про неясное обращение:\n%s", in)
	}
	if strings.Contains(in, "уточни,") {
		t.Errorf("в clarify не должно быть служебного «уточни»:\n%s", in)
	}
}

// Вся авторская проза дела обращается к игроку на «вы»; «ты» посреди неё
// читается как другой голос. Живой прогон дал и то и другое в одном экране.
func TestNarratePinsTheFormOfAddress(t *testing.T) {
	m, f := masterWith(t, "проза")
	m.Narrate(context.Background(), KindPlace, "рамка", World{}, nil, "", nil, llm.Request{})
	if sys := f.Calls()[0].System; !strings.Contains(sys, "«вы»") {
		t.Errorf("форма обращения не закреплена:\n%s", sys)
	}
}

// Отказ мира произносит Мастер — но переформулирует, а не решает: текст
// отказа даёт ядро, Мастер одевает его в язык мира. Игрок должен слышать
// собеседника, а не служебную строку.
func TestRefuseSpeaksTheWorldsLanguage(t *testing.T) {
	m, f := masterWith(t, "Смотритель весов качает головой: об этом здесь не говорят.")

	out, err := m.Refuse(context.Background(), "здесь об этом не расскажут",
		World{Setting: "посёлок в устье", Scene: []string{"Место: Пристань"}}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "не говорят") {
		t.Errorf("переформулировка не доехала: %q", out)
	}
	call := f.Calls()[0]
	if call.Role != llm.RoleNarrator {
		t.Errorf("отказ озвучен не ролью Мастера: %q", call.Role)
	}
	// Одна строка не стоит дорогой модели — ровно та же причина, что у Grant.
	if call.Tier != llm.TierCheap {
		t.Errorf("отказ ушёл дорогим тиром: %q", call.Tier)
	}
	if !strings.Contains(call.Input, "здесь об этом не расскажут") {
		t.Errorf("Мастеру не дали текст отказа:\n%s", call.Input)
	}
}

// Мастер видит ТОЛЬКО текст отказа и сцену. Требований гейта ядро наружу не
// отдаёт вовсе, и подсказать, чем путь открыть, Мастеру структурно нечем —
// это и держит постановление «отказ гейта не подсказывает».
func TestRefuseIsToldNothingAboutWhatWouldOpenThePath(t *testing.T) {
	m, f := masterWith(t, "Дверь не поддаётся.")
	if _, err := m.Refuse(context.Background(), "туда пока незачем идти",
		World{Scene: []string{"Место: Пристань"}}, llm.Request{}); err != nil {
		t.Fatal(err)
	}
	body := f.Calls()[0].System + f.Calls()[0].Input
	for _, leak := range []string{"requires", "гейт", "unlock", "f_"} {
		if strings.Contains(body, leak) {
			t.Errorf("в промпт отказа утекло %q:\n%s", leak, body)
		}
	}
}

// Пустой отказ модель не беспокоит: вызов без нужды — деньги за шум.
func TestRefuseSkipsEmptyRefusal(t *testing.T) {
	m, f := masterWith(t, "не должно быть вызвано")
	out, err := m.Refuse(context.Background(), "  ", World{}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("пустой отказ что-то вернул: %q", out)
	}
	if len(f.Calls()) != 0 {
		t.Errorf("модель вызвана впустую: %d вызовов", len(f.Calls()))
	}
}

// Брифинг — единственное место, где игрок обязан получить факты дела дословно.
// Мастер его рассказывает, но не досочиняет: имя, место, время и число здесь
// авторские, и добавить своё значит соврать игроку о деле с первой же строки.
func TestBriefingPromptForbidsAddingCaseContent(t *testing.T) {
	m, f := masterWith(t, "Магистрат прислал вас в Гавань.")

	out, err := m.Narrate(context.Background(), KindBriefing,
		"Вас прислал магистрат. Тело Халдена нашли на складе.",
		World{Setting: "посёлок в устье"}, nil, "", nil, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("брифинг не рассказан")
	}
	system := strings.ToLower(f.Calls()[0].System)
	for _, must := range []string{"не добавляй", "имён", "чисел"} {
		if !strings.Contains(system, must) {
			t.Errorf("в промпте брифинга нет запрета про %q:\n%s", must, system)
		}
	}
	// Место и исход — другие работы, и путать их нельзя: на брифинге игрок
	// ещё ничего не видел и ничего не делал.
	if strings.Contains(f.Calls()[0].Input, "ОПИСАНИЕ МЕСТА") ||
		strings.Contains(f.Calls()[0].Input, "ИСХОД") {
		t.Errorf("брифинг подан как место или исход:\n%s", f.Calls()[0].Input)
	}
}

// Пустая рамка означает, что автор брифинга не написал: тогда придумывать его
// Мастеру нечем и незачем.
func TestBriefingWithoutFrameIsSkipped(t *testing.T) {
	m, f := masterWith(t, "не должно быть вызвано")
	out, err := m.Narrate(context.Background(), KindBriefing, "  ", World{}, nil, "", nil, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if out != "" || len(f.Calls()) != 0 {
		t.Errorf("пустой брифинг дошёл до модели: %q, вызовов %d", out, len(f.Calls()))
	}
}

// Проба — не исход, и инструкция обязана быть обратной. У исхода Мастер
// показывает, чем кончилось; у пробы — что ничем не кончилось: ни находки, ни
// намёка на то, где искать (ADR-0003, T2).
func TestProbeProseIsForbiddenToConclude(t *testing.T) {
	m, f := masterWith(t, "Сети пахнут тиной.")
	if _, err := m.Narrate(context.Background(), KindProbe, "рамка", World{},
		[]string{"Игрок пробует: ковыряет ворох сетей"}, "", nil, llm.Request{}); err != nil {
		t.Fatal(err)
	}
	in := f.Calls()[0].Input
	if !strings.Contains(in, "СВОБОДНАЯ ПРОБА") {
		t.Errorf("проба подана Мастеру не как проба:\n%s", in)
	}
	if !strings.Contains(in, "Ничем НЕ кончай") {
		t.Errorf("Мастеру не запрещено доводить пробу до исхода:\n%s", in)
	}
	if !strings.Contains(in, "ковыряет ворох сетей") {
		t.Errorf("сама проба до Мастера не доехала:\n%s", in)
	}
}

// --- consistency-guard прозы: Мастер не вправе соврать про состояние ---

// Проза с ложным состоянием чинится тем же переспросом, что и реплика актёра:
// первый прогон соврал, ремонт сказал то же без лжи — его и печатаем.
func TestNarrateStateContradictionIsRepaired(t *testing.T) {
	m, _ := masterWith(t, "проза")
	ch := &stubChecker{verdicts: []bool{false, true}, what: "ты уже в кузнице"}
	m = m.WithGuard(ch)

	out, err := m.Narrate(context.Background(), KindOutcome, "рамка",
		World{Scene: []string{"Место: Пристань"}}, []string{"исход: успех"}, "",
		[]string{"Сейчас находится: Пристань"}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Error("ремонт прошёл, а проза пропала")
	}
	if ch.calls != 2 {
		t.Errorf("проверок %d — отремонтированная проза не перепроверена", ch.calls)
	}
}

// Повтор противоречия — проза откатывается на авторскую рамку: Narrate
// возвращает пусто, и презентация печатает рамку, которая про состояние не
// врёт. Нейтраль здесь — доверенный текст автора, а не заглушка.
func TestNarratePersistentContradictionFallsToFrame(t *testing.T) {
	m, _ := masterWith(t, "проза, врущая про состояние")
	m = m.WithGuard(&stubChecker{verdicts: []bool{false, false}, what: "ты уже в кузнице"})

	out, err := m.Narrate(context.Background(), KindPlace, "рамка", World{},
		nil, "", []string{"Сейчас находится: Пристань"}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("проза, противоречащая состоянию, дошла до игрока: %q", out)
	}
}

// Гвард видит и прозу, и дайджест состояния, и авторскую рамку: проверить
// противоречие можно только сверяя одно с другим.
func TestNarrateGuardSeesStateAndMaterial(t *testing.T) {
	m, _ := masterWith(t, "проза")
	ch := &stubChecker{}
	m = m.WithGuard(ch)

	m.Narrate(context.Background(), KindOutcome, "рамка автора",
		World{Scene: []string{"Место: Пристань"}}, []string{"исход: успех"}, "",
		[]string{"Несёт при себе: предписание"}, llm.Request{})

	if !strings.Contains(strings.Join(ch.stateSeen, " | "), "предписание") {
		t.Errorf("дайджест состояния не дошёл до гварда: %v", ch.stateSeen)
	}
	if !strings.Contains(strings.Join(ch.materialSeen, " | "), "рамка автора") {
		t.Errorf("авторская рамка не попала в материал гварда: %v", ch.materialSeen)
	}
}

// Пустой дайджест — проверки нет: брифинг (единственное место, где игроку
// легально сообщают факты дела) идёт со State=nil, и гвард его не трогает.
func TestNarrateWithoutStateSkipsGuard(t *testing.T) {
	m, _ := masterWith(t, "брифинг")
	ch := &stubChecker{verdicts: []bool{false}} // позовут — зарубит
	m = m.WithGuard(ch)

	out, err := m.Narrate(context.Background(), KindBriefing, "рамка", World{},
		nil, "", nil, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if out != "брифинг" {
		t.Errorf("проза без дайджеста зарублена гвардом: %q", out)
	}
	if ch.calls != 0 {
		t.Errorf("гвард позван при пустом состоянии: вызовов %d", ch.calls)
	}
}

// Ремонт прозы называет, чего утверждать нельзя: переспрос точнее, когда назван
// конкретный ложный факт о состоянии.
func TestNarrateRepairNamesTheContradiction(t *testing.T) {
	m, f := masterWith(t, "проза")
	m = m.WithGuard(&stubChecker{verdicts: []bool{false, true}, what: "ты уже в кузнице"})

	m.Narrate(context.Background(), KindOutcome, "рамка", World{},
		[]string{"исход: успех"}, "", []string{"Сейчас находится: Пристань"}, llm.Request{})

	if len(f.Calls()) < 2 {
		t.Fatalf("ремонт не переспросил Мастера: вызовов %d", len(f.Calls()))
	}
	if in := f.Calls()[1].Input; !strings.Contains(in, "ты уже в кузнице") {
		t.Errorf("ремонт не назвал противоречие состоянию:\n%s", in)
	}
}
