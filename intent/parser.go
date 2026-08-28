package intent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/store"
)

// Result — исход разбора. Ровно одно из трёх: действие, свободная проба или
// уточняющий вопрос. Исхода-тупика нет: приземляется всё (ADR-0003, T1).
type Result struct {
	Intent *core.Intent
	// Clarify — вопрос игроку. Парсер договаривается на границе словаря,
	// а не отбивает ввод: отказ без предложения и есть та клетка, которой
	// боятся в закрытом словаре.
	Clarify string
	// Candidate — ввод, не покрытый словарём. Кандидат в новый глагол.
	// Игроку не показывается: это сигнал метрике о том, где словарь узок.
	Candidate string
	// Probe — свободная проба: что игрок пробует, когда словарь такого не
	// выражает. Не отказ, а приземление (ADR-0003, T1): отклик описывает
	// Мастер, а если проба легла на авторскую цель — её резолвит ядро.
	Probe string
	// Class — класс предложенного глагола, пуст если глагол не предлагался.
	Class core.VerbClass
	// Reply — безоценочная реплика Мастера, пришедшая тем же вызовом. Только
	// в чат-режиме; в обычном разборе её никто не просит. Об исходе она не
	// знает и знать не может — бросок ещё не сделан.
	Reply string
}

func (r Result) Accepted() bool { return r.Intent != nil }

type Parser struct {
	gw      *llm.Gateway
	metrics *Metrics
	// chat — режим одного вызова: модель разбирает фразу и тем же ответом
	// отвечает игроку. Флаг, а не отдельный тип: разбор и проверка значений
	// у обоих режимов одни и те же, и вторая копия validate разъехалась бы с
	// первой молча.
	chat bool
}

func NewParser(gw *llm.Gateway) *Parser {
	return &Parser{gw: gw, metrics: NewMetrics()}
}

// NewChatParser — парсер чат-режима: один вызов даёт и разбор, и реплику
// Мастера. Эксперимент рядом с основным путём, а не вместо него.
func NewChatParser(gw *llm.Gateway) *Parser {
	return &Parser{gw: gw, metrics: NewMetrics(), chat: true}
}

func (p *Parser) Metrics() *Metrics { return p.metrics }

const systemPrompt = `Ты переводишь фразу игрока в действие настольной игры.

Правила, которые нельзя нарушать:
1. Глагол выбирается ТОЛЬКО из перечисленных в схеме. Своих не придумывай.
2. target, topic и node — это идентификаторы ИЗ СПИСКА сцены ниже. Ничего,
   чего в списке нет, использовать нельзя: такой сущности в мире не существует.
3. Вопрос человеку — это question ИЛИ ask_about, и различие важное:
   - question — вопрос о ДЕЛЕ: о теле, о шнуре, о той ночи, о том, кто где был.
     Тема НЕОБЯЗАТЕЛЬНА и по умолчанию ПУСТА: указывай её только если игрок
     спросил именно об этом факте. Совпадение слова с формулировкой факта
     темой не делает, и единственная известная тема не становится темой
     оттого, что других нет.
   - ask_about — вопрос о МИРЕ: где поесть, где переждать воду, кто держит
     весовую, как тут вообще живут, что за место. К делу это не относится, и
     фактов тут не выдают — человек просто отвечает, а чего не знает, о том
     спросит того, кто знает.
   Сомневаешься, о деле вопрос или о мире, — ask_about: спросить о мире можно
   всегда, а question на неподходящем вопросе даёт отказ, и игрок остаётся
   вообще без ответа. Спросил открыто («что
   слышно?», «кто убийца?», «расскажи про ту ночь») — question без темы, и
   человек расскажет то, что готов рассказать. Не отказывай и не переводи это
   в say: словарь тут ни при чём.
   - examine и search тему тоже принимают: «осмотреть шнур на теле» — это
     examine с темой. Тема только из списка известных; не знаешь — общий осмотр.
4. Если фраза не ложится ни на один глагол, верни free_probe и опиши в probe,
   что игрок пробует. Отказа не бывает: игрок пробует поисследовать, и мир
   отвечает откликом, а не сообщением о том, что так нельзя. Не подгоняй фразу
   под неподходящий глагол — но и не отбивай её.
   - Если игрок при этом описывает физическое действие, добавь class — на что
     оно похоже по ФОРМЕ: attack для удара и возни, skill для ловкости и
     точной работы руками, move для перемещения. Это подсказка о форме, и
     только: сложность назначает игра, и назвать действие лёгким или трудным
     ты не можешь ничем — ни этим полем, ни словами в probe. Не уверен в форме
     — оставь class пустым, пустой лучше выдуманного.
5. Если фраза ложится, но непонятно на что именно, верни clarify с коротким
   вопросом — в характере мира, не служебным языком. Уточнение — это вопрос
   игроку, а не отказ, и второй раз одно и то же не спрашивают: если тот же
   вопрос напрашивается снова, это free_probe.
6. clarify и probe читает ИГРОК между репликами: одна короткая фраза, без
   спора с ним и без объяснений, почему он неправ. Пиши языком мира, а не
   языком словаря:
   «бумаг при себе не оказалось», а не «в доступных действиях нет глагола для
   использования предметов». О схеме, глаголах, списках и полях игрок не знает
   и знать не должен.

Ты не решаешь, удалось ли действие. Ты только переводишь.

Обязательные аргументы по глаголам:
%s
Примеры. Обрати внимание: обязательное поле заполняется ВСЕГДА, даже когда
игрок называет персонажа по имени, а не по идентификатору.

Сцена: e_bern — Берн, стражник
Игрок: «Поздороваться с Берном»
Ответ: {"outcome":"intent","verb":"talk_to","target":"e_bern"}

Детали места: p_barrels — Штабель бочек
Игрок: «осмотреть бочки»
Ответ: {"outcome":"intent","verb":"examine","target":"p_barrels"}

Сцена: e_ivar — Ивар, кузнец; известные темы: f_ledger — гроссбух
Игрок: «спрошу кузнеца про книгу»
Ответ: {"outcome":"intent","verb":"question","target":"e_ivar","topic":"f_ledger"}

Сцена: e_ivar — Ивар, кузнец; известных тем нет
Игрок: «спрошу кузнеца, кто убийца»
Ответ: {"outcome":"intent","verb":"question","target":"e_ivar"}

Сцена: e_nils — Нильс, посыльный; известные темы: f_body_found — тело нашли на складе у пристани
Игрок: «прошу указать дорогу до склада»
Ответ: {"outcome":"intent","verb":"ask_about","target":"e_nils"}
Это вопрос о мире, не о деле: игрок спросил дорогу. Темы нет, хотя слово
«склад» есть и в вопросе, и в формулировке факта — совпадение слова не есть
совпадение темы, и единственную известную тему нельзя ставить только потому,
что она единственная.

Сцена: e_nils — Нильс, посыльный
Игрок: «а где тут можно поесть?»
Ответ: {"outcome":"intent","verb":"ask_about","target":"e_nils"}
Темы нет, хотя слово «склад» есть и в вопросе, и в формулировке факта. Игрок
спросил дорогу, а не про тело: совпадение слова — не совпадение темы. Ставить
единственную известную тему только потому, что она единственная, нельзя.

Разговор идёт с e_bern
Игрок: «есть ли какие-нибудь слухи в последнее время?»
Ответ: {"outcome":"intent","verb":"question","target":"e_bern"}

Разговор идёт с e_bern
Игрок: «достаю из кармана»
Ответ: {"outcome":"clarify","clarify":"Что вы достаёте?"}

При себе: i_writ — Предписание магистрата; сцена: e_bern — Берн, стражник
Игрок: «показать предписание Берну»
Ответ: {"outcome":"intent","verb":"present","item":"i_writ","target":"e_bern"}

Детали места: p_barrels — Штабель бочек
Игрок: «принюхиваюсь, чем тут пахнет за бочками»
Ответ: {"outcome":"free_probe","probe":"принюхивается к воздуху за штабелем бочек"}
Ни один глагол этого не выражает — и это не повод отказывать. Игрок пробует,
мир отвечает. Формы у принюхивания нет — class пуст.

Детали места: p_door — Трухлявая дверь
Игрок: «наваливаюсь плечом на дверь»
Ответ: {"outcome":"free_probe","probe":"наваливается плечом на дверь","class":"attack"}
Форма понятна — это возня и сила. Насколько это трудно, решает игра.`

// chatAddendum — то, чем чат-режим отличается от разбора: разобрав фразу,
// модель тем же ответом отвечает игроку. Добавка, а не свой промпт: правила
// разбора у режимов одни, и второй их экземпляр разъехался бы с первым.
const chatAddendum = `

Кроме разбора ты ОТВЕЧАЕШЬ игроку — голосом Мастера, в поле reply.

Реплика безоценочная. Ты не знаешь, чем кончится действие: бросок ещё не
сделан, а решает его не ты. Поэтому НЕ УТВЕРЖДАЙ ИСХОД — ни успеха, ни
провала, ни того, что игрок нашёл, услышал или разглядел. Покажи только
начало: как он тянется, наклоняется, поворачивается, обращается. Чем оно
кончилось, игрок прочтёт следующей строкой.

  Игрок: «осмотреть бочки»
  reply: «Вы наклоняетесь к штабелю, ведя рукой по сырой клёпке.»
  НЕ: «Под верхней бочкой обнаруживается обрывок шнура.» — это исход.

За людей не говори: тот, к кому обращён ход, ответит сам, своей репликой,
следующей строкой. Ни его слов, ни его тона, ни того, смягчился он или
насторожился.

Одна-две фразы, на «вы», языком мира. Ни чисел, ни броска, ни служебных
пометок: механику печатает не ты.`

// Parse переводит текст в интент. Возвращает ошибку только на отказе шлюза
// или сломанном ответе; непонятый ввод — это Result, а не ошибка.
func (p *Parser) Parse(ctx context.Context, text string, hint SceneHint, req llm.Request) (Result, error) {
	res, repair, err := p.attempt(ctx, text, hint, req, "")
	if err != nil {
		return Result{}, err
	}
	// Один раунд починки. Слабая модель игнорирует инструкцию об обязательном
	// поле, но исправляется, когда ей называют пропущенное. Второго раунда нет
	// намеренно: дальше это уже не недопонимание, а неподходящий глагол.
	if repair != "" {
		res, _, err = p.attempt(ctx, text, hint, req, repair)
		if err != nil {
			return Result{}, err
		}
	}
	p.observe(res)
	return res, nil
}

func (p *Parser) attempt(ctx context.Context, text string, hint SceneHint,
	req llm.Request, repair string) (Result, string, error) {
	req.Role = llm.RoleIntentParser
	req.Schema = schemaJSONFor(hint)
	req.System = fmt.Sprintf(systemPrompt, arityBrief())
	if p.chat {
		req.Role = llm.RoleChatMaster
		req.Schema = chatSchemaJSONFor(hint)
		req.System += chatAddendum
	}
	req.Input = "Сцена:\n" + hint.Render() + "\nИгрок пишет: " + text +
		"\n\nЕсли действие направлено на кого-то или что-то из сцены — ОБЯЗАТЕЛЬНО заполни " +
		"target его идентификатором. Игрок называет цель своими словами и в своём падеже " +
		"(«бочки» это p_barrels); сопоставь сам."
	if repair != "" {
		req.Input += "\n\nПредыдущий ответ был неполон: " + repair +
			"\nВерни тот же глагол, заполнив пропущенное значением из сцены."
	}

	resp, err := p.gw.Do(ctx, req)
	if err != nil {
		return Result{}, "", err
	}
	var raw reply
	if err := json.Unmarshal([]byte(resp.Text), &raw); err != nil {
		return Result{}, "", fmt.Errorf("intent: ответ не разобрался: %w", err)
	}
	res, repairNext := p.validate(raw, hint, text)
	// Реплика проверке значений не подлежит: она не ссылается на сцену, а
	// описывает то, что игрок только что сделал. Пустая — законный исход:
	// надстройка не должна быть условием работы хода.
	res.Reply = strings.TrimSpace(raw.Reply)
	return res, repairNext, nil
}

// observe записывает метрику один раз по окончательному результату: иначе
// раунд починки считался бы отказом дважды.
func (p *Parser) observe(res Result) {
	p.metrics.Observe(res.Class, outcomeOf(res))
}

// outcomeOf переводит результат в исход для метрики. Три значения, а не два:
// проба — не отказ и не принятие. Считать её непонятым вводом значило бы
// сломать единственный сигнал о том, где словарь действительно узок, — а
// приземляется теперь всё, и доля «непонятого» иначе поехала бы в единицу.
func outcomeOf(res Result) Observed {
	switch {
	case res.Accepted():
		return ObservedAccepted
	case res.Probe != "":
		return ObservedProbe
	default:
		return ObservedRejected
	}
}

// probeText — что игрок пробует. Доводчик берёт его собственные слова: модель
// поле не обязана заполнять, а описывать пробу бледно лучше, чем никак.
func probeText(raw reply, text string) string {
	return fallback(strings.TrimSpace(raw.Probe), strings.TrimSpace(text))
}

// probeClass — предложенная моделью форма свободной пробы, проверенная по
// реестру ЯДРА. Выдуманное значение не «неизвестный класс», а отсутствие
// подсказки: принять его на слово значило бы дать модели заводить категории в
// домене. Ни сложности, ни легальности подсказка не несёт — их решает ядро.
func probeClass(raw reply) core.VerbClass {
	c, ok := core.LookupClass(strings.TrimSpace(raw.Class))
	if !ok {
		return ""
	}
	return c
}

// validate — вторая половина гарантии. Схема отвечает за форму ответа, эта
// функция за его смысл: ссылка на сущность вне сцены отклоняется, даже если
// формально валидна.
// validate возвращает результат и, если ответ можно починить одним уточнением,
// текст этого уточнения. Метрику здесь не пишем: она считается по итогу.
func (p *Parser) validate(raw reply, hint SceneHint, text string) (Result, string) {
	switch raw.Outcome {
	case OutcomeClarify:
		return Result{Clarify: fallback(raw.Clarify, "уточни, что именно ты делаешь")}, ""
	case OutcomeFreeProbe:
		return Result{Probe: probeText(raw, text), Class: probeClass(raw)}, ""
	case OutcomeUnsupported:
		// Приземляется пробой, но сигнал о узости словаря сохраняется: метрика
		// меряет именно его, и без него не видно, ГДЕ словарь узок.
		return Result{Probe: probeText(raw, text), Class: probeClass(raw),
			Candidate: fallback(raw.Reason, "словарь такого не покрывает")}, ""
	case OutcomeIntent:
	default:
		return Result{Clarify: "не разобрал, повтори иначе"}, ""
	}

	def, ok := core.LookupVerb(raw.Verb)
	if !ok {
		// Глагола нет в реестре — класс неизвестен, значит это не отказ
		// конкретного класса, а промах модели по схеме.
		return Result{Clarify: "не разобрал, повтори иначе"}, ""
	}

	reject := func(msg string) (Result, string) {
		return Result{Clarify: msg, Class: def.Class}, ""
	}

	// Прежде чем спрашивать, попробуем разрешить ссылку сами. Игрок почти
	// всегда называет персонажа по имени, а поиск имени в сцене — это
	// подстрока, а не суждение. Один сэкономленный вызов и, что важнее,
	// ход не превращается в допрос игрока о том, что он только что написал.
	need := requires(def.Verb)
	if need.Target && raw.Target == "" {
		if id, ok := resolveByName(text, hint.targets()); ok {
			raw.Target = id
		}
	}
	// Разговор продолжается с тем же человеком: называть его в каждой фразе
	// игрок не обязан, и структурированный ввод это давно умеет. Собеседник
	// подставляется, только если он всё ещё в сцене — иначе игрок обратился
	// бы к тому, кто ушёл.
	if need.Target && raw.Target == "" && hint.Talk.With != "" &&
		hint.hasEntity(string(hint.Talk.With)) {
		raw.Target = string(hint.Talk.With)
	}
	// Социальный ход обращён к человеку. Адресата арность может и не
	// требовать — предъявление формально бывает и без цели, — но названный
	// словами адресат обязан разрешаться, иначе «показать предписание Берну»
	// уходит в отказ «предъявлять некому» при названном Берне.
	if raw.Target == "" && def.Class == core.ClassSocial {
		if id, ok := resolveByName(text, hint.Entities); ok {
			raw.Target = id
		} else if with := string(hint.Talk.With); with != "" && hint.hasEntity(with) {
			raw.Target = with
		}
	}
	// Предмет игрок называет словами: «показать предписание». Поиск имени в
	// сцене — подстрока, а не суждение, и без него ход превращался в допрос
	// игрока о том, что он только что написал.
	if need.Item && raw.Item == "" {
		if id, ok := resolveByName(text, itemChoices(hint)); ok {
			raw.Item = id
		}
	}
	// Тему игрок часто называет словами: «спрошу про тело на складе». Поиск
	// имени в фразе — подстрока, а не суждение. Названная тема ценнее
	// открытого вопроса: она спрашивает именно о том, о чём хотели, поэтому
	// ищется даже там, где тема необязательна.
	if raw.Topic == "" && topicGuessableFromText(def.Verb) {
		if id, ok := resolveByName(text, hint.Topics); ok {
			raw.Topic = id
		}
	}
	if need.Node && raw.Node == "" {
		if id, ok := resolveByName(text, hint.Reachable); ok {
			raw.Node = id
		}
	}
	// Там, где обязательный аргумент — свободный текст, он уже произнесён:
	// это сама фраза игрока. Спрашивать «что именно ты хочешь сказать?» у
	// того, кто только что это сказал, — допрос игрока о его же строке, и
	// живой прогон упирался в него дважды за три хода.
	if need.Text && strings.TrimSpace(raw.Text) == "" {
		raw.Text = strings.TrimSpace(text)
	}

	if msg := requires(def.Verb).missing(raw); msg != "" {
		// Уточнение про цель обязано её назвать. Модель target почти никогда
		// не заполняет сама — для людей это незаметно, их имя находится в
		// фразе, а для деталей места не находится из-за падежей. Игрок при
		// этом видит бочки в списке сцены и не понимает, каким словом в них
		// попасть.
		if need.Target && raw.Target == "" {
			if names := targetNames(hint); names != "" {
				msg = "к кому или к чему? здесь: " + names
			}
		}
		// Пропущенный обязательный аргумент — единственный случай, который
		// стоит починить: глагол угадан, не хватает ссылки на сцену.
		return Result{Clarify: msg, Class: def.Class}, "не заполнено обязательное поле — " + msg
	}
	// Аргумент, которого глагол не берёт, до ядра не доходит. Модель со строгой
	// схемой заполняет поля, потому что они в required, а не потому что они
	// нужны: живой прогон дал node в КАЖДОМ интенте, включая question и
	// talk_to, и тему дела в вопросе о мире. Пользы от такого аргумента нет
	// никакой, а провалить ход он может — validate проверяет по узлу смежность
	// и запертость, и разговор отвергается из-за запертого склада.
	if !takesTopic(def.Verb) {
		raw.Topic = ""
	}
	if !requires(def.Verb).Node {
		raw.Node = ""
	}
	if raw.Target != "" && !hint.hasEntity(raw.Target) {
		return reject("этого здесь нет — кого ты имеешь в виду?")
	}
	if raw.Topic != "" && !hint.hasTopic(raw.Topic) {
		return reject("парти об этом ещё ничего не знает")
	}
	if raw.Node != "" && !hint.hasNode(raw.Node) {
		return reject("туда отсюда не пройти")
	}
	for _, f := range raw.Facts {
		if !hint.hasTopic(f) {
			return reject("такого факта парти не знает")
		}
	}

	in := &core.Intent{Verb: def.Verb, Args: core.Args{
		Target:  store.EntityID(raw.Target),
		Topic:   store.FactID(raw.Topic),
		Node:    store.NodeID(raw.Node),
		Item:    raw.Item,
		Ability: raw.Ability,
		Text:    raw.Text,
	}}
	for _, f := range raw.Facts {
		in.Args.Facts = append(in.Args.Facts, store.FactID(f))
	}

	return Result{Intent: in, Class: def.Class}, ""
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// targetNames — имена всего, на что можно указать, через запятую.
func targetNames(hint SceneHint) string {
	all := hint.targets()
	names := make([]string, 0, len(all))
	for _, t := range all {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}

// takesTopic — глаголы, которые тему принимают: обязательно или нет. Открытый
// вопрос законен, но названная тема всё равно лучше, и упустить её нельзя.
// examine и search тему тоже принимают: «искать конкретное» — законный ход, а
// гейт знания стоит в ядре и срабатывает сам.
//
// ask_about здесь НЕТ, и это не пропуск: вопрос о мире темы дела не имеет по
// определению. Пока он тут стоял, единственная известная тема приезжала в
// вопрос про дорогу — и ядро отказывало по ней.
func takesTopic(v core.Verb) bool {
	switch v {
	case "question", "cross_reference", "examine", "search":
		return true
	}
	return false
}

// topicGuessableFromText — глаголы, для которых подстрочный доводчик темы
// (resolveByName по hint.Topics) уместен. У question и cross_reference тема —
// предмет вопроса, и промах дешёв: без темы вопрос остаётся открытым, просто
// не таким прицельным. У examine и search тема СУЖАЕТ поиск — она меняет,
// какого держателя ищет ядро (см. holderFor), а имена фактов — это фразы
// («Тело сборщика податей Халдена найдено на складе у пристани»), и
// naming.Mentions ловит любое их значимое слово. «Осмотреть тело» на складе
// «Гавани» подставляло topic=f_body_found по слову «тело» из имени факта — и
// первый же осмысленный ход свободным текстом превращался в отказ вместо
// находки. Поэтому у examine и search тема остаётся только той, которую
// модель назвала явно через схему: takesTopic эти глаголы не покидает — явная
// тема должна оставаться выразимой, — а доводчик по подстроке для них просто
// не запускается.
func topicGuessableFromText(v core.Verb) bool {
	switch v {
	case "question", "cross_reference":
		return true
	}
	return false
}

// bareName — ввод целиком, называющий одного из присутствующих. Строго одно
// слово: «Берн, что слышно?» это уже фраза, и разбирать её должна модель.
func bareName(text string, entities []Named) (string, bool) {
	if len(strings.Fields(strings.TrimSpace(text))) != 1 {
		return "", false
	}
	return resolveByName(text, entities)
}

// GameInterpreter связывает парсер с идущей игрой: подсказка собирается на
// каждый ввод, потому что сцена меняется. Реализует интерфейс переводчика,
// которого ждёт CLI, — структурно, без импорта презентации.
type GameInterpreter struct {
	Parser *Parser
	Game   *core.Game
	// Req несёт идентификаторы плательщика и ход; потолки шлюза считаются
	// по ним.
	Req llm.Request
}

// InterpretChat — тот же разбор, что Interpret, плюс безоценочная реплика
// Мастера тем же вызовом. Реализует интерфейс чат-режима, которого ждёт CLI, —
// структурно, без импорта презентации.
//
// Дешёвые перехваты работают и здесь: одно слово-имя это по-прежнему обращение,
// и платить за него вызовом незачем. Реплики у такого хода нет — её заменяет
// ответ самого человека, который сейчас и заговорит.
func (gi *GameInterpreter) InterpretChat(ctx context.Context, text string,
	with store.EntityID, pending string) (*core.Intent, string, core.Probe, string, error) {
	return gi.interpret(ctx, text, with, pending)
}

func (gi *GameInterpreter) Interpret(ctx context.Context, text string, with store.EntityID,
	pending string) (*core.Intent, core.Probe, string, error) {
	in, _, probe, clarify, err := gi.interpret(ctx, text, with, pending)
	return in, probe, clarify, err
}

// interpret — общее тело обоих путей. Реплика возвращается всегда, а
// показывает её только чат-режим: одно место разбора вместо двух, которые
// разъехались бы молча.
func (gi *GameInterpreter) interpret(ctx context.Context, text string, with store.EntityID,
	pending string) (in *core.Intent, reply string, probe core.Probe, clarify string, err error) {
	hint := BuildHint(gi.Game)
	// Одно слово — имя того, к кому игрок повернулся. Самый дешёвый ход в
	// разговоре, и вызов модели ему не нужен: имя в сцене это подстрока, а не
	// суждение. Живой прогон отвечал на «Берн» отказом — модель искала
	// действие там, где действие было очевидно.
	if id, ok := bareName(text, hint.Entities); ok {
		// Реплики у такого хода нет, и придумывать её незачем: сейчас
		// заговорит сам человек, к которому повернулись.
		return &core.Intent{Verb: "talk_to", Actor: gi.Game.Actor,
			Args: core.Args{Target: store.EntityID(id), Text: text}}, "", core.Probe{}, "", nil
	}
	// Разговор — часть сцены. Транскрипт лежит в дневнике собеседника: его
	// ведёт озвучка, а разбор им пользуется, и второго места правды не
	// появляется.
	hint.Talk = Talk{With: with, Pending: pending}
	if with != "" {
		hint.Talk.Recent = gi.Game.D.Recent(with)
	}
	res, err := gi.Parser.Parse(ctx, text, hint, gi.Req)
	if err != nil {
		return nil, "", core.Probe{}, "", err
	}
	switch {
	case res.Accepted():
		// Слова игрока обязаны доехать до персонажа. talk_to и прочие
		// социальные глаголы своего текста не несут, и без этого актёр
		// получал пустую реплику: «поздороваться с Берном» приходило к нему
		// как молчание, и он отвечал не на приветствие, а ни на что.
		//
		// Там, где текст и есть содержание хода — theorize, say, emote, —
		// разобранное моделью важнее исходной строки: в дневник пишется
		// гипотеза, а не команда её записать.
		if res.Intent.Args.Text == "" {
			res.Intent.Args.Text = text
		}
		return res.Intent, res.Reply, core.Probe{}, "", nil
	case res.Probe != "":
		// Приземление вместо тупика. Раньше здесь стоял отказ словарём, и
		// именно он рубил исследование: игрок пробовал, а мир отвечал
		// служебным языком, что так нельзя.
		return nil, res.Reply, core.Probe{Text: res.Probe, Class: res.Class}, "", nil
	default:
		return nil, "", core.Probe{}, res.Clarify, nil
	}
}
