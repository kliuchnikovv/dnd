// TS-зеркало Go-контракта view/turnview.go (rev.2) — ЕДИНСТВЕННЫЙ источник формы вида.
// Отзеркалено вербатим по json-тегам. Правила паритета:
//   - Go-указатель (*T)        → optional поле (T | undefined)
//   - слайс с ",omitempty"     → optional массив
//   - bool без omitempty       → ОБЯЗАТЕЛЬНОЕ поле (Meter.surface, Panel.surface — всегда в JSON)
// При изменении view/turnview.go — обновить этот файл и мок-фикстуры (guard-тест паритета словит).
//
// Клиент рисует ДЕСКРИПТОРЫ, не конкретику: Meter/Resolution называет ПРАВИЛО, Panel — СЦЕНАРИЙ.
// Никаких «grit»/«d20»/«casebook» в коде рендерера (guard агностичности).

/** Типизированный вид одного хода — единственное, что уходит наружу. */
export interface TurnView {
    version: number;
    scene: Scene;
    /** Проза Мастера и реплики NPC. Стрим: механика собрана мгновенно, проза доезжает токенами. */
    narration?: Block[];
    /** Исход броска. Отсутствует у безопасного действия. */
    resolution?: Resolution;
    /** Состояние от ПРАВИЛА. Что показать — решает сервер флагом surface. */
    meters?: Meter[];
    options?: Option[];
    /** Панель цели от СЦЕНАРИЯ (досье — её экземпляр). Скрыта по умолчанию. */
    objective?: Panel;
    map?: MapView;
    participants?: Who[];
    /** Развязка. Пока игра идёт — отсутствует. */
    ended?: Ending;
}

/** Где мы. art_ref — id ассета (заглушка до асс-стора). */
export interface Scene {
    node: string;
    title: string;
    art_ref?: string;
}

/** Единица прозы. kind: "gm" (Мастер) | "npc" (реплика). Проза — контент, не разметка. */
export interface Block {
    kind: string;
    text: string;
    speaker?: Who;
    streaming?: boolean;
}

/** Мера состояния от ПРАВИЛА. kind — машинный вид ("harm"/"grit"/"clock"); label — слово игроку. */
export interface Meter {
    label: string;
    kind: string;
    value: number;
    max?: number;
    /** Всегда присутствует. Клиент рисует только meters с surface=true. */
    surface: boolean;
}

/** Исход броска без словаря конкретной кости. */
export interface Resolution {
    terms?: Term[];
    target: number;
    margin: number;
    outcome: Outcome;
}

/** Слагаемое броска словами игрока: «расследование +2». */
export interface Term {
    label: string;
    value: number;
}

/** Класс исхода. tier — машинная ступень ("fail"|"partial"|"success"|"crit"); label — слово игроку. */
export interface Outcome {
    label: string;
    tier: string;
}

/** Предложенный ход. token — непрозрачный id (клиент шлёт обратно, сервер валидирует).
 *  check — класс проверки словами, НЕ порог. reply — это реплика, а не действие. */
export interface Option {
    id: string;
    label: string;
    token: string;
    check?: string;
    reply?: boolean;
}

/** Присутствующий. avatar_ref — id ассета (заглушка). */
export interface Who {
    id: string;
    name: string;
    avatar_ref?: string;
    disposition: number;
}

/** Карта при показе. Узлы несут только то, что парти вправе знать. */
export interface MapView {
    art_ref?: string;
    nodes?: MapNode[];
}

export interface MapNode {
    id: string;
    name: string;
    reachable: boolean;
    known: boolean;
    x: number;
    y: number;
}

/** Обобщённая панель цели от СЦЕНАРИЯ. kind — архетип ("deduction"). Клиент рендерит обобщённо. */
export interface Panel {
    kind: string;
    title: string;
    sections?: Section[];
    /** Всегда присутствует. Скрыта по умолчанию — вызывается жестом. */
    surface: boolean;
}

export interface Section {
    label: string;
    items?: PanelItem[];
    slots?: Slot[];
}

/** Строка панели. token — непрозрачная ссылка для жеста над строкой. */
export interface PanelItem {
    id: string;
    label: string;
    sub?: string;
    token?: string;
}

/** Пустая ячейка рабочего пространства. filled — чем занята; options — чем можно занять. */
export interface Slot {
    name: string;
    label: string;
    filled?: PanelItem;
    options?: PanelItem[];
}

/** Развязка. kind различает раскрытое дело и висяк; text — сценарный исход. */
export interface Ending {
    kind: string;
    text?: string;
}
