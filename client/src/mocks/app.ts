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
