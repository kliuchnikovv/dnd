package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

// presentGame — держатель, который отдаёт факт только предъявившему бумагу.
// Годная бумага — это власть, а не проверка навыка, поэтому броска нет.
func presentGame() *Game {
	g := turnGame(OutcomeFail) // исход броска нарочно провальный: он не должен участвовать
	g.DB.Items["i_writ"] = store.Item{ID: "i_writ", Kind: "credential",
		Name: "Предписание магистрата", DispositionDelta: 1}
	g.DB.Items["i_boot"] = store.Item{ID: "i_boot", Kind: "evidence", Name: "Сапог"}
	g.DB.Facts["f_writ_only"] = store.Fact{ID: "f_writ_only", Key: "по бумаге"}
	g.DB.Holders["f_writ_only"] = []store.FactHolder{{
		FactID: "f_writ_only", HolderID: "e_toke",
		Gate: store.Gate{Verbs: []string{"present", "question"}, Threshold: "normal",
			RequiresItems: []store.ItemID{"i_writ"}},
	}}
	return g
}

func present(g *Game, item store.ItemID, target store.EntityID) TurnResult {
	return g.Apply(Intent{Verb: "present", Actor: g.Actor,
		Args: Args{Target: target, Item: string(item)}})
}

// Годный credential открывает гейт без броска и теплит расположение.
func TestPresentCredentialOpensGateAndWarmsDisposition(t *testing.T) {
	g := presentGame()
	g.Acquire("i_writ")

	before := g.D.Disposition("e_toke")
	got := present(g, "i_writ", "e_toke")
	if got.Refused {
		t.Fatalf("предъявление отказано: %s", got.Refusal)
	}
	if len(got.Learned) != 1 || got.Learned[0].Fact != "f_writ_only" {
		t.Fatalf("факт не выдан: %+v", got.Learned)
	}
	if got.Res != nil {
		t.Error("предъявление бросило кость — годная бумага это не проба")
	}
	if after := g.D.Disposition("e_toke"); after != before+1 {
		t.Errorf("расположение %d, было %d: авторский сдвиг не применён", after, before)
	}
}

// Не тот предмет гейт не открывает, и подсказки об этом нет: иначе перебор
// предметов становится способом вычитать граф дела.
func TestPresentingWrongItemOpensNothing(t *testing.T) {
	g := presentGame()
	g.Acquire("i_boot")

	got := present(g, "i_boot", "e_toke")
	if got.Refused {
		t.Fatalf("предъявление своего предмета отказано: %s", got.Refusal)
	}
	if len(got.Learned) != 0 {
		t.Errorf("факт выдан не тем предметом: %+v", got.Learned)
	}
	if g.K.Knows("f_writ_only") {
		t.Error("парти узнала гейтнутый факт мимо гейта")
	}
}

// Предъявить то, чего не несёшь, — отказ, а не провал: ход не потрачен.
func TestPresentWithoutCarryingIsRefused(t *testing.T) {
	g := presentGame()
	turnsBefore := g.Attempts

	got := present(g, "i_writ", "e_toke")
	if !got.Refused {
		t.Fatal("предъявлено то, чего парти не несёт")
	}
	if len(got.Learned) != 0 || g.K.Knows("f_writ_only") {
		t.Error("отказ всё равно выдал факт")
	}
	if g.Attempts != turnsBefore {
		t.Error("отказ потратил ход")
	}
}

// Предмета, которого нет в деле, предъявить нельзя — это опечатка, не действие.
func TestPresentUnknownItemIsRefused(t *testing.T) {
	if got := present(presentGame(), "i_нет_такого", "e_toke"); !got.Refused {
		t.Error("предъявлен предмет, которого нет в деле")
	}
}

// Сдвиг расположения — один на предмет, как рассказанная тема. Иначе бумагу
// показывают десять раз и получают дружбу из ничего.
func TestDispositionShiftsOncePerItem(t *testing.T) {
	g := presentGame()
	g.Acquire("i_writ")

	present(g, "i_writ", "e_toke")
	after := g.D.Disposition("e_toke")
	present(g, "i_writ", "e_toke")
	present(g, "i_writ", "e_toke")

	if got := g.D.Disposition("e_toke"); got != after {
		t.Errorf("расположение доросло до %d за три предъявления одной бумаги", got)
	}
}

// Предъявление — видимое действие с адресатом. Предъявление узлу формой
// оставлено на будущее, но в MVP его нет, и молчать об этом нельзя.
func TestPresentWithoutTargetIsRefused(t *testing.T) {
	g := presentGame()
	g.Acquire("i_writ")
	if got := present(g, "i_writ", ""); !got.Refused {
		t.Error("предъявление в воздух прошло как действие")
	}
}

// Гейт помнит, КОМУ предъявили: бумага, показанная одному, не открывает
// чужой гейт. Иначе «предъявление» становится пассивным пропуском.
func TestPresentationIsRememberedPerHolder(t *testing.T) {
	g := presentGame()
	g.DB.Holders["f_writ_only"] = append(g.DB.Holders["f_writ_only"], store.FactHolder{
		FactID: "f_writ_only", HolderID: "e_ivar",
		Gate: store.Gate{Verbs: []string{"question"}, Threshold: "normal",
			RequiresItems: []store.ItemID{"i_writ"}},
	})
	g.Acquire("i_writ")
	present(g, "i_writ", "e_toke")

	if !g.D.Presented("e_toke", "i_writ") {
		t.Error("предъявление не запомнилось")
	}
	if g.D.Presented("e_ivar", "i_writ") {
		t.Error("предъявление одному сошло за предъявление другому")
	}
}

// Гейт на предмет закрыт для тех глаголов, которых в нём нет: предъявление
// не отменяет грамматику гейта.
func TestItemGateStillHonoursVerbList(t *testing.T) {
	g := presentGame()
	g.Acquire("i_writ")
	present(g, "i_writ", "e_toke")
	g.K = NewKnowledge(g.DB) // забыть узнанное, оставив память о предъявлении

	got := g.Apply(Intent{Verb: "ask_about", Actor: g.Actor,
		Args: Args{Target: "e_toke", Topic: "f_writ_only"}})
	if len(got.Learned) != 0 {
		t.Errorf("факт выдан глаголом, которого нет в гейте: %+v", got.Learned)
	}
}

// Предмет приходит вместе с фактом: нашёл нож — получил улику и знание разом.
// Иначе автор дела обязан выдавать предметы отдельной механикой, которой нет.
func TestLearningFactGrantsItsItem(t *testing.T) {
	g := presentGame()
	g.DB.Items["i_knife"] = store.Item{ID: "i_knife", Kind: "evidence", Name: "Нож"}
	g.DB.Facts["f_open"] = store.Fact{ID: "f_open", Key: "open", GrantsItem: "i_knife"}

	if g.Carries("i_knife") {
		t.Fatal("предмет в инвентаре до того, как факт узнан")
	}
	got := g.Apply(Intent{Verb: "question", Actor: g.Actor,
		Args: Args{Target: "e_toke", Topic: "f_open"}})
	if len(got.Learned) != 1 {
		t.Fatalf("факт не узнан: %+v", got)
	}
	if !g.Carries("i_knife") {
		t.Error("факт узнан, а предмет не выдан")
	}
}

// Выдача идёт и на пути вывода: факт, открытый сопоставлением, приносит свой
// предмет так же, как факт, узнанный от человека.
func TestFactRevealedByCompareGrantsItsItem(t *testing.T) {
	g := presentGame()
	g.DB.Items["i_knife"] = store.Item{ID: "i_knife", Kind: "evidence", Name: "Нож"}
	g.DB.Facts["f_third"] = store.Fact{ID: "f_third", Key: "третий", GrantsItem: "i_knife"}
	g.DB.Contradictions = append(g.DB.Contradictions, store.Contradiction{
		A: "f_open", B: "f_gated", Reveals: "f_third"})
	g.K.Learn("f_open", "e_toke")
	g.K.Learn("f_gated", "e_toke")

	if got := g.Compare("f_open", "f_gated"); len(got.Learned) != 1 {
		t.Fatalf("сопоставление не открыло факт: %+v", got)
	}
	if !g.Carries("i_knife") {
		t.Error("факт открыт выводом, а предмет не выдан")
	}
}

// Факт, выдающий предмет, которого нет в деле, ничего не портит: инвентарь не
// место, где вещи появляются из воздуха.
func TestFactGrantingUnknownItemGrantsNothing(t *testing.T) {
	g := presentGame()
	g.DB.Facts["f_open"] = store.Fact{ID: "f_open", Key: "open", GrantsItem: "i_нет_такого"}
	got := g.Apply(Intent{Verb: "question", Actor: g.Actor,
		Args: Args{Target: "e_toke", Topic: "f_open"}})
	if len(got.Learned) != 1 {
		t.Fatalf("факт не узнан: %+v", got)
	}
	if g.Carries("i_нет_такого") {
		t.Error("в инвентарь попал предмет, которого нет в деле")
	}
}

// Инструментальность живёт в одном поле на две таблицы: носимый инструмент
// берётся в руки так же, как проп узла. Пока их было две — тег у пропа и вид у
// предмета, — они успели разойтись в данных.
func TestCarriedToolIsReadiedLikeAProp(t *testing.T) {
	g := presentGame()
	g.DB.Items["i_lantern"] = store.Item{ID: "i_lantern", Kind: store.ToolKind, Name: "Фонарь"}
	g.Acquire("i_lantern")

	if tools := g.SceneView(Intent{Verb: "examine"}).Tools; len(tools) != 0 {
		t.Fatalf("носимый инструмент активен без хода: %v", tools)
	}
	if res := g.Apply(Intent{Verb: "use_item", Args: Args{Item: "i_lantern"}}); res.Refused {
		t.Fatalf("взять фонарь не вышло: %s", res.Refusal)
	}
	if tools := g.SceneView(Intent{Verb: "examine"}).Tools; len(tools) != 1 {
		t.Errorf("носимый инструмент не попал в сцену: %v", tools)
	}
}

// Предмет не вида tool инструментом не становится, сколько его ни применяй:
// иначе предъявляемая бумага отменяла бы штраф среды.
func TestNonToolItemIsNotAToolCurrent(t *testing.T) {
	g := presentGame()
	g.Acquire("i_writ")
	g.Apply(Intent{Verb: "use_item", Args: Args{Item: "i_writ"}})
	if tools := g.SceneView(Intent{Verb: "examine"}).Tools; len(tools) != 0 {
		t.Errorf("бумага сработала как инструмент: %v", tools)
	}
}
