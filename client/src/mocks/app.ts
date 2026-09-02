// App-level мок-модели для экранов оболочки (Хаб, Лента Мира, Создание персонажа).
// У них ПОКА НЕТ серверного контракта (в отличие от turn-view) — это локальные view-модели,
// плейсхолдеры до соответствующих серверных спек. Держим их отдельно от turnview/types.ts,
// чтобы не путать с боевым контрактом.

// ── Хаб ────────────────────────────────────────────────────────────────────────
export interface HubModel {
    heroName: string;
    heroReputation: string;
    priority: {
        caseTitle: string;
        line: string;
        cta: string;
    };
    worldDigest: { id: string; text: string; role: 'wound' | 'moss' | 'faint' }[];
}

export const havenHub: HubModel = {
    heroName: 'Дознаватель',
    heroReputation: 'на хорошем счету у магистрата',
    priority: {
        caseTitle: 'Гавань · «Три Кита»',
        line: 'Берн ответил на ваше предписание, пока вас не было. Мастер ждёт вашего хода.',
        cta: 'сделать ход',
    },
    worldDigest: [
        { id: 'w1', text: 'В Нижнем порту сорвана печать с таможенного склада', role: 'wound' },
        { id: 'w2', text: 'Странники подтвердили пропажу «Соляной звезды»', role: 'moss' },
        { id: 'w3', text: 'Магистрат объявил награду за сведения о верфи', role: 'faint' },
    ],
};

// ── Лента Мира ───────────────────────────────────────────────────────────────
export interface WorldEvent {
    id: string;
    kind: 'ambient' | 'hook';
    icon: string; // ключ иконки
    text: string;
    meta: string; // «регион · когда»
    // Только у хука: действие ведёт к интенту/делу.
    action?: { label: string; token: string };
}

export const havenWorldFeed: WorldEvent[] = [
    {
        id: 'e1',
        kind: 'hook',
        icon: 'scale',
        text: 'Магистрат вынес вердикт по делу «Соляной звезды» — виновным назван корабельщик Гест. Кто-то в порту знает, что это ложь.',
        meta: 'Гавань · только что',
        action: { label: 'взять на опровержение', token: 'world_refute_gest' },
    },
    {
        id: 'e2',
        kind: 'ambient',
        icon: 'fire',
        text: 'Странники сожгли брошенный пакгауз у третьего причала, чтобы выкурить крыс.',
        meta: 'Гавань · вчера',
    },
    {
        id: 'e3',
        kind: 'ambient',
        icon: 'anchor',
        text: 'Прилив поднялся выше отметки — нижние мостки затоплены до утра.',
        meta: 'Гавань · этой ночью',
    },
];

// ── Создание персонажа (ruleset-driven: правило «Порог») ──────────────────────
export interface Archetype {
    id: string;
    name: string;
    blurb: string;
    // Дескрипторы статов из правила (label от правила, не хардкод клиента).
    stats: { label: string; value: number }[];
    tags: string[];
    token: string;
}

export interface RulesetCreation {
    ruleName: string;
    statBudget: number; // сумма раздачи
    statMax: number;
    archetypes: Archetype[];
}

export const thresholdCreation: RulesetCreation = {
    ruleName: 'Порог',
    statBudget: 4,
    statMax: 3,
    archetypes: [
        {
            id: 'a.inspector',
            name: 'Инспектор',
            blurb: 'Читает бумаги и людей. Там, где другие видят беспорядок, вы видите пропущенную строку.',
            stats: [
                { label: 'внимание', value: 3 },
                { label: 'убеждение', value: 1 },
            ],
            tags: ['архив', 'протокол'],
            token: 'archetype_inspector',
        },
        {
            id: 'a.enforcer',
            name: 'Пристав',
            blurb: 'Ваше слово тяжелее засова. Разговор идёт быстрее, когда за ним стоит закон.',
            stats: [
                { label: 'убеждение', value: 2 },
                { label: 'напор', value: 2 },
            ],
            tags: ['магистрат', 'нажим'],
            token: 'archetype_enforcer',
        },
        {
            id: 'a.tracker',
            name: 'Следопыт',
            blurb: 'Порт врёт словами, но не следами. Вы идёте туда, куда ведёт грязь на сапогах.',
            stats: [
                { label: 'внимание', value: 2 },
                { label: 'выносливость', value: 2 },
            ],
            tags: ['улицы', 'слежка'],
            token: 'archetype_tracker',
        },
        {
            id: 'a.fixer',
            name: 'Посредник',
            blurb: 'У вас везде есть должник. Дверь, закрытая для закона, открыта для вас.',
            stats: [
                { label: 'убеждение', value: 3 },
                { label: 'внимание', value: 1 },
            ],
            tags: ['связи', 'долги'],
            token: 'archetype_fixer',
        },
    ],
};

// ── Создание персонажа (ruleset-driven: правило «D&D 5e») ─────────────────────
// Статы архетипов — полный набор из шести характеристик D&D 5e (Str/Dex/Con/Int/Wis/Cha),
// в отличие от «Порога», где стат-лист собирается из ролевых дескрипторов.
export const dnd5eCreation: RulesetCreation = {
    ruleName: 'D&D 5e',
    statBudget: 72,
    statMax: 16,
    archetypes: [
        {
            id: 'fighter',
            name: 'Файтер',
            blurb: 'Тяжёлая броня и твёрдая рука. Там, где план рушится, вы держите строй.',
            stats: [
                { label: 'Str', value: 16 },
                { label: 'Dex', value: 12 },
                { label: 'Con', value: 14 },
                { label: 'Int', value: 8 },
                { label: 'Wis', value: 10 },
                { label: 'Cha', value: 10 },
            ],
            tags: ['броня', 'меч'],
            token: 'archetype_fighter',
        },
        {
            id: 'rogue',
            name: 'Рог',
            blurb: 'Тень между тенями. Кинжал решает то, что не решил разговор.',
            stats: [
                { label: 'Str', value: 10 },
                { label: 'Dex', value: 16 },
                { label: 'Con', value: 12 },
                { label: 'Int', value: 12 },
                { label: 'Wis', value: 10 },
                { label: 'Cha', value: 8 },
            ],
            tags: ['скрытность', 'кинжал'],
            token: 'archetype_rogue',
        },
        {
            id: 'ranger',
            name: 'Рейнджер',
            blurb: 'Дикие тропы читаются, как чужие письма. Лук бьёт раньше, чем вас заметят.',
            stats: [
                { label: 'Str', value: 10 },
                { label: 'Dex', value: 15 },
                { label: 'Con', value: 12 },
                { label: 'Int', value: 10 },
                { label: 'Wis', value: 14 },
                { label: 'Cha', value: 8 },
            ],
            tags: ['лук', 'следопыт'],
            token: 'archetype_ranger',
        },
        {
            id: 'cleric',
            name: 'Клерик',
            blurb: 'Слово бога тяжелее булавы. Щит держит удар, молитва — исход.',
            stats: [
                { label: 'Str', value: 12 },
                { label: 'Dex', value: 8 },
                { label: 'Con', value: 14 },
                { label: 'Int', value: 10 },
                { label: 'Wis', value: 16 },
                { label: 'Cha', value: 12 },
            ],
            tags: ['божественное', 'щит'],
            token: 'archetype_cleric',
        },
    ],
};

// rulesets — каталог правил, доступных при создании персонажа (шаг выбора
// ruleset перед архетипом). Порядок — в порядке появления в продукте.
export const rulesets = [thresholdCreation, dnd5eCreation];
