// Package view — то единственное, что уходит наружу на ход. Клиент (RN) и
// мультиплеер висят на одном контракте: что сервер отдаёт игроку каждый ход.
// Сервера ещё нет, но контракт вводится уже сейчас — Go-структурой, которую
// консоль умеет заполнять и печатать, а сервер потом сериализует в JSON без
// изменения формы (ADR-0004).
//
// Вид собран из ДЕСКРИПТОРОВ, не из конкретики, и это две оси пластичности:
// клиент не завязан ни на набор правил (D&D/«Порог» — одна реализация
// core.RuleSystem), ни на сценарий (детектив — один архетип). Меры и форму
// резолюции называет ПРАВИЛО; панель цели даёт СЦЕНАРИЙ; клиент рисует
// дескрипторы, не зная слов «grit», «casebook» или «d20». D&D-детектив —
// референс-конфигурация, наполняющая дескрипторы, а не форма самого вида.
//
// Пакет лежит НАД доменом: он импортирует core, но core о нём не знает —
// граница ядра цела (architecture_test).
package view

// TurnView — типизированный вид одного хода. Единственное, что уходит наружу.
//
// Здесь по построению нет ни cases.truth, ни гейтнутого-неизвестного факта:
// Options и Objective собираются из того же read-scope, что core.Affordances.
type TurnView struct {
	// Version — версия формы. Клиент и сервер расходятся в развитии, и вид
	// обязан нести собственный номер, а не выводиться из версии сборки.
	Version int   `json:"version"`
	Scene   Scene `json:"scene"`
	// Narration — проза Мастера и реплики NPC. Стрим: доезжает токенами уже
	// после того, как механика (Meters/Options/Resolution) собрана из ядра
	// мгновенно. Клиент рисует механику сразу, прозу — по мере прихода.
	Narration []Block `json:"narration,omitempty"`
	// Resolution — исход броска. Указатель: у безопасного действия броска нет,
	// и вид не тащит по проводу пустую резолюцию.
	Resolution *Resolution `json:"resolution,omitempty"`
	// Meters — состояние от ПРАВИЛА. Что из них показать сейчас, решает сервер
	// флагом Surface, а не эвристика клиента.
	Meters  []Meter  `json:"meters,omitempty"`
	Options []Option `json:"options,omitempty"`
	// Objective — панель цели от СЦЕНАРИЯ. Указатель и скрыта по умолчанию:
	// детективное досье это её экземпляр, подземелье пришлёт другую.
	Objective *Panel   `json:"objective,omitempty"`
	Map       *MapView `json:"map,omitempty"`
	// Participants — присутствующие, каждый со ссылкой на аватар.
	Participants []Who `json:"participants,omitempty"`
	// Ended — развязка. Условие победы сценарное: раскрыл дело / зачистил /
	// ушёл с добычей. Указатель: пока игра идёт, его нет.
	Ended *Ending `json:"ended,omitempty"`
}

// Scene — где мы. ArtRef — id ассета, заглушка до асс-стора: вид ссылается на
// картинку, но её самой не несёт.
type Scene struct {
	Node   string `json:"node"`
	Title  string `json:"title"`
	ArtRef string `json:"art_ref,omitempty"`
}

// Block — единица прозы. Kind различает голос Мастера («gm») и реплику
// персонажа («npc»): клиент по нему решает оформление, но по тексту не
// навигирует — проза это контент в контейнере, а не разметка.
type Block struct {
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	Speaker   *Who   `json:"speaker,omitempty"`
	Streaming bool   `json:"streaming,omitempty"`
}

// Meter — мера состояния, названная ПРАВИЛОМ. Kind — машинный вид («harm»,
// «grit», «clock»), Label — слово правила для игрока. Max опционален: у раны он
// есть, у некоторых мер нет. Surface решает сервер: клиент рисует только то,
// что всплыло.
type Meter struct {
	Label   string `json:"label"`
	Kind    string `json:"kind"`
	Value   int    `json:"value"`
	Max     *int   `json:"max,omitempty"`
	Surface bool   `json:"surface"`
}

// Resolution — исход броска в форме, свободной от словаря конкретной кости.
// «Порог» с d20 приходит сюда как термы, цель и исход; клиент рисует их, не
// зная про d20.
type Resolution struct {
	Terms   []Term  `json:"terms,omitempty"`
	Target  int     `json:"target"`
	Margin  int     `json:"margin"`
	Outcome Outcome `json:"outcome"`
}

// Term — слагаемое броска словами игрока: «расследование +2».
type Term struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

// Outcome — класс исхода. Label — слово игроку, Tier — машинная ступень
// («fail»|«partial»|«success»|«crit») для клиента, который красит исход, но
// русских слов не разбирает.
type Outcome struct {
	Label string `json:"label"`
	Tier  string `json:"tier"`
}

// Option — предложенный ход, привязанный к интенту. Label — слова презентации;
// Token — непрозрачный id, который клиент шлёт обратно, а сервер разворачивает
// в интент и ВСЁ РАВНО валидирует. Check — класс проверки словами, а НЕ порог:
// число живёт в гейте держателя, и напечатать его значило бы разметить, у каких
// целей есть авторский контент (ADR-0003, T2). Reply — это реплика, а не
// действие: клиент по нему решает, брать ли строку в кавычки.
type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Token string `json:"token"`
	Check string `json:"check,omitempty"`
	Reply bool   `json:"reply,omitempty"`
}

// Who — присутствующий. AvatarRef — id ассета, заглушка до асс-стора.
type Who struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AvatarRef   string `json:"avatar_ref,omitempty"`
	Disposition int    `json:"disposition"`
}

// MapView — карта при её показе. Узлы несут только то, что парти вправе знать:
// достижимость и известность — из read-scope, не из графа мира.
type MapView struct {
	ArtRef string    `json:"art_ref,omitempty"`
	Nodes  []MapNode `json:"nodes,omitempty"`
}

type MapNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Reachable bool   `json:"reachable"`
	Known     bool   `json:"known"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
}

// Panel — обобщённая панель цели от СЦЕНАРИЯ. Детективное досье — её экземпляр:
// факты с источниками и confidence ложатся в Sections, слоты обвинения
// who/how/when/why — в Slots. Клиент рендерит обобщённую панель, не «casebook».
// Kind называет архетип («deduction»); Surface скрывает её по умолчанию —
// вызывается жестом.
type Panel struct {
	Kind     string    `json:"kind"`
	Title    string    `json:"title"`
	Sections []Section `json:"sections,omitempty"`
	Surface  bool      `json:"surface"`
}

type Section struct {
	Label string      `json:"label"`
	Items []PanelItem `json:"items,omitempty"`
	Slots []Slot      `json:"slots,omitempty"`
}

// PanelItem — строка панели: факт+источник+confidence у детектива, что-то иное
// у другого сценария. Token — ссылка для жеста над строкой (заполнить слот и
// т.п.), непрозрачная так же, как у Option.
type PanelItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Sub   string `json:"sub,omitempty"`
	Token string `json:"token,omitempty"`
}

// Slot — пустая ячейка рабочего пространства: who/how/when/why у детектива и
// аналоги у другого сценария. Filled — чем занята, если занята; Options — чем
// её можно занять.
type Slot struct {
	Name    string      `json:"name"`
	Label   string      `json:"label"`
	Filled  *PanelItem  `json:"filled,omitempty"`
	Options []PanelItem `json:"options,omitempty"`
}

// Ending — развязка. Kind различает раскрытое дело и висяк; Text — сценарный
// текст исхода. Условие победы сценарное, поэтому и живёт здесь, а не в ядре.
type Ending struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
}
