import { TurnView, Who } from '../../turnview/types';
import { MockSource } from '../../turnview/source';

// Мок-фикстуры дела «Гавань» (таверна «Три Кита»). Наполняют контрактные дескрипторы так же,
// как это сделала бы реф-конфигурация cli/refview.go (правило «Порог» × сценарий-детектив):
// меры harm/grit/clock, панель "deduction" «Досье», слоты who/how/when/why, факты с источниками.
//
// Фикстуры типизированы как TurnView — tsc проверяет паритет с view/turnview.go на компиляции.
// Токены (Option.token / PanelItem.token) непрозрачны; переходы между видами держит таблица ниже.

const bern: Who = { id: 'npc.bern', name: 'Берн', avatar_ref: 'av/bern', disposition: -1 };
const player: Who = { id: 'pc', name: 'Вы', disposition: 0 };

// ── Сцена: прибытие ────────────────────────────────────────────────────────────
export const sceneArrival: TurnView = {
    version: 1,
    scene: { node: 'harbor.tavern', title: 'Три Кита', art_ref: 'scene/tavern-night' },
    narration: [
        {
            kind: 'gm',
            text: 'Фонарь качается над стойкой. Портовая таверна пахнет смолой и остывшей ухой. Хозяин, Берн, вытирает кружку и смотрит мимо вас — слишком старательно, чтобы это было случайностью.',
        },
    ],
    meters: [
        { label: 'grit', kind: 'grit', value: 2, surface: true },
        // harm=0 → surface:false: рендерер обязан это скрыть (зеркало cli/render.go).
        { label: 'ранения', kind: 'harm', value: 0, max: 4, surface: false },
        { label: 'прилив', kind: 'clock', value: 4, max: 6, surface: true },
    ],
    options: [
        { id: 'o.talk', label: 'заговорить с Берном', token: 'talk_bern' },
        { id: 'o.ledger', label: 'осмотреть журнал у стойки', token: 'examine_ledger', check: 'внимание' },
        { id: 'o.map', label: 'выйти на пристань', token: 'open_map' },
    ],
    participants: [bern],
};

// ── Сцена: осмотр журнала (с резолюцией) ───────────────────────────────────────
export const sceneLedger: TurnView = {
    version: 1,
    scene: { node: 'harbor.tavern', title: 'Три Кита', art_ref: 'scene/tavern-night' },
    resolution: {
        terms: [
            { label: 'внимание', value: 2 },
            { label: 'бросок', value: 11 },
        ],
        target: 12,
        margin: 1,
        outcome: { label: 'с осложнением', tier: 'partial' },
    },
    narration: [
        {
            kind: 'gm',
            text: 'В журнале не хватает страницы за прошлую субботу — вырвана торопливо. Но по вдавленному следу на листе ниже читается имя: «Кассир». Берн замечает ваш интерес, и его рука замирает на кружке.',
        },
    ],
    meters: [
        { label: 'grit', kind: 'grit', value: 2, surface: true },
        { label: 'прилив', kind: 'clock', value: 5, max: 6, surface: true },
    ],
    options: [
        { id: 'o.talk', label: 'заговорить с Берном', token: 'talk_bern' },
        { id: 'o.map', label: 'выйти на пристань', token: 'open_map' },
        { id: 'o.dossier', label: 'открыть досье', token: 'open_dossier' },
    ],
    participants: [bern],
};

// ── Диалог: Берн ───────────────────────────────────────────────────────────────
export const dialogueBern: TurnView = {
    version: 1,
    scene: { node: 'harbor.tavern', title: 'Разговор · Берн', art_ref: 'scene/tavern-night' },
    narration: [
        {
            kind: 'npc',
            text: 'Нечего тут ходить по ночам. Кассир? Не знаю никакого кассира. Забирайте свою любознательность и ступайте спать.',
            speaker: bern,
        },
    ],
    options: [
        { id: 'o.press', label: 'Страница вырвана. Кто был здесь в субботу?', token: 'press_bern', reply: true },
        { id: 'o.persuade', label: 'надавить, что укрывательство — тоже статья', token: 'persuade_bern', check: 'убеждение' },
        { id: 'o.doc', label: 'показать предписание магистрата', token: 'show_doc' },
        { id: 'o.back', label: 'отступить к стойке', token: 'back_scene' },
    ],
    participants: [bern, player],
};

// ── Диалог: Берн отвечает на нажим (стриминг) ──────────────────────────────────
export const dialogueBernPressed: TurnView = {
    version: 1,
    scene: { node: 'harbor.tavern', title: 'Разговор · Берн', art_ref: 'scene/tavern-night' },
    narration: [
        {
            kind: 'npc',
            text: 'В субботу? Был один. С верфи. Просил не записывать — и заплатил, чтобы не записывал. Больше ничего не скажу, слышите. Ничего.',
            speaker: bern,
            streaming: true,
        },
    ],
    options: [
        { id: 'o.persuade', label: 'имя. сейчас же', token: 'persuade_bern', check: 'убеждение' },
        { id: 'o.doc', label: 'показать предписание магистрата', token: 'show_doc' },
        { id: 'o.back', label: 'отступить к стойке', token: 'back_scene' },
    ],
    participants: [bern, player],
};

// ── Диалог: проверка «убеждение» — резолюция провала ───────────────────────────
export const dialogueBernPersuade: TurnView = {
    version: 1,
    scene: { node: 'harbor.tavern', title: 'Разговор · Берн', art_ref: 'scene/tavern-night' },
    resolution: {
        terms: [
            { label: 'убеждение', value: 1 },
            { label: 'бросок', value: 6 },
        ],
        target: 13,
        margin: -6,
        outcome: { label: 'провал', tier: 'fail' },
    },
    narration: [
        {
            kind: 'npc',
            text: 'Берн отворачивается и уходит в подсобку. Замок щёлкает. Разговор окончен — вы передавили.',
            speaker: bern,
        },
    ],
    meters: [
        { label: 'прилив', kind: 'clock', value: 6, max: 6, surface: true },
    ],
    options: [
        { id: 'o.back', label: 'вернуться к стойке', token: 'back_scene' },
        { id: 'o.map', label: 'выйти на пристань', token: 'open_map' },
    ],
    participants: [bern, player],
};

// ── Карта: пристань ────────────────────────────────────────────────────────────
export const mapHarbor: TurnView = {
    version: 1,
    scene: { node: 'harbor', title: 'Пристань', art_ref: 'map/harbor' },
    map: {
        art_ref: 'map/harbor-skin',
        nodes: [
            { id: 'harbor.tavern', name: 'Три Кита', reachable: false, known: true, x: 30, y: 70 },
            { id: 'harbor.docks', name: 'Причалы', reachable: true, known: true, x: 55, y: 45 },
            { id: 'harbor.warehouse', name: 'Склад', reachable: true, known: true, x: 78, y: 60 },
            { id: 'harbor.shipyard', name: 'Верфь', reachable: false, known: true, x: 70, y: 22 },
            // Туман: известен факт существования, но не место.
            { id: 'harbor.fog', name: '?', reachable: false, known: false, x: 90, y: 30 },
        ],
    },
    meters: [
        { label: 'прилив', kind: 'clock', value: 5, max: 6, surface: true },
    ],
    options: [
        { id: 'o.warehouse', label: 'идти к складу', token: 'move_warehouse' },
        { id: 'o.docks', label: 'идти к причалам', token: 'move_docks' },
        { id: 'o.back', label: 'вернуться в таверну', token: 'back_scene' },
    ],
    participants: [],
};

// ── Досье: панель дедукции ─────────────────────────────────────────────────────
export const dossier: TurnView = {
    version: 1,
    scene: { node: 'harbor.tavern', title: 'Три Кита', art_ref: 'scene/tavern-night' },
    objective: {
        kind: 'deduction',
        title: 'Досье',
        surface: true,
        sections: [
            {
                label: 'Факты',
                items: [
                    { id: 'f.page', label: 'Страница за субботу вырвана', sub: '0.800 ← журнал' },
                    { id: 'f.cashier', label: 'Имя «Кассир» вдавлено на листе', sub: '0.500 ← журнал' },
                    { id: 'f.shipyard', label: 'Гость был с верфи, платил за молчание', sub: '* 0.900 ← Берн, журнал' },
                ],
            },
            {
                label: 'Обвинение',
                slots: [
                    {
                        name: 'who', label: 'Кто',
                        options: [
                            { id: 't.bern', label: 'Берн', token: 'fill_who_bern' },
                            { id: 't.cashier', label: 'Кассир', token: 'fill_who_cashier' },
                        ],
                    },
                    {
                        name: 'how', label: 'Как',
                        filled: { id: 't.bribe', label: 'подкуп за молчание' },
                    },
                    { name: 'when', label: 'Когда', options: [{ id: 't.sat', label: 'суббота', token: 'fill_when_sat' }] },
                    // Пустой без опций — заблокирован: нет открытых токенов.
                    { name: 'why', label: 'Почему' },
                ],
            },
            {
                label: 'Гипотезы',
                items: [{ id: 'h.1', label: 'Кассир с верфи откупился, чтобы скрыть груз' }],
            },
        ],
    },
    options: [
        { id: 'o.back', label: 'закрыть досье', token: 'back_scene' },
    ],
    participants: [],
};

// ── Все виды + таблица переходов ───────────────────────────────────────────────
export const havenViews: Record<string, TurnView> = {
    sceneArrival,
    sceneLedger,
    dialogueBern,
    dialogueBernPressed,
    dialogueBernPersuade,
    mapHarbor,
    dossier,
};

export const havenTransitions: Record<string, string> = {
    talk_bern: 'dialogueBern',
    examine_ledger: 'sceneLedger',
    open_map: 'mapHarbor',
    open_dossier: 'dossier',
    press_bern: 'dialogueBernPressed',
    persuade_bern: 'dialogueBernPersuade',
    show_doc: 'dialogueBernPressed',
    back_scene: 'sceneArrival',
    move_warehouse: 'sceneArrival',
    move_docks: 'sceneArrival',
};

export function createHavenSource(start = 'sceneArrival'): MockSource {
    return new MockSource({ views: havenViews, start, transitions: havenTransitions });
}
