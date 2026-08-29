import { create } from 'zustand';

import { TurnViewSource } from '../turnview/source';
import { createHavenSource } from '../mocks/haven/case';
import { TurnView } from '../turnview/types';

export type TabKey = 'home' | 'world' | 'case' | 'hero' | 'more';

// Сессионный под-экран выводится ИЗ ДАННЫХ вида (какие поля заполнены), а не из интерпретации
// непрозрачного токена — клиент остаётся тонким: рисует то, что прислано.
export type SessionMode = 'scene' | 'dialogue' | 'map' | 'dossier';

export function sessionModeOf(view: TurnView): SessionMode {
    if (view.map) return 'map';
    if (view.objective?.surface) return 'dossier';
    // Диалог: игрок «внутри» разговора (присутствует как участник pc).
    if ((view.participants ?? []).some((w) => w.id === 'pc')) return 'dialogue';
    return 'scene';
}

interface AppState {
    tab: TabKey;
    setTab: (tab: TabKey) => void;
    source: TurnViewSource;
    setSource: (source: TurnViewSource) => void;
}

// source по умолчанию — createHavenSource() (дев/офлайн-фикстура). Прод-путь
// для игры подменяет его на NetSource (SessionScreen-контейнер, Task 7) через
// setSource() при выборе/старте сессии.
export const useAppStore = create<AppState>((set) => ({
    tab: 'home',
    setTab: (tab) => set({ tab }),
    source: createHavenSource(),
    setSource: (source) => set({ source }),
}));
