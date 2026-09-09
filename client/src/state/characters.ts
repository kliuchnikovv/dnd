import { create } from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';
import * as Crypto from 'expo-crypto';

// Ruleset — код правила, которым создан персонаж. Определяет, какой набор
// архетипов/статов показывать (см. client/src/mocks/app.ts: rulesets).
// "vignette" — виньеточный трек (ADR-0009): лист персонажа не читается,
// ruleset тут — просто гейт совместимости персонаж↔дело на POST /sessions
// (см. server/server.go: handleCreateSession).
export type Ruleset = 'threshold' | 'dnd5e' | 'vignette';

// CharacterRecord — минимальная запись персонажа на клиенте: только то, что
// нужно для выбора при старте сессии (POST /sessions {case_id, character_id}).
// Полную модель листа персонажа сервер отдаёт по character_id отдельно.
export type CharacterRecord = {
  id: string;
  name: string;
  ruleset: Ruleset;
  archetypeId: string;
};

const STORAGE_KEY = 'dnd.characters.v1';

interface CharactersState {
  characters: CharacterRecord[];
  // add — создаёт персонажа, сохраняет в AsyncStorage и добавляет в стор.
  add: (name: string, ruleset: Ruleset, archetypeId: string) => Promise<CharacterRecord>;
  // getById — синхронный поиск по уже загрученному списку (без обращения к хранилищу).
  getById: (id: string) => CharacterRecord | undefined;
  // hydrate — загружает сохранённых персонажей из AsyncStorage при старте приложения.
  hydrate: () => Promise<void>;
}

async function persist(characters: CharacterRecord[]): Promise<void> {
  await AsyncStorage.setItem(STORAGE_KEY, JSON.stringify(characters));
}

export const useCharacters = create<CharactersState>((set, get) => ({
  characters: [],

  async add(name, ruleset, archetypeId) {
    const character: CharacterRecord = {
      id: Crypto.randomUUID(),
      name,
      ruleset,
      archetypeId,
    };
    const next = [...get().characters, character];
    set({ characters: next });
    await persist(next);
    return character;
  },

  getById(id) {
    return get().characters.find((c) => c.id === id);
  },

  async hydrate() {
    try {
      const raw = await AsyncStorage.getItem(STORAGE_KEY);
      if (!raw) return;
      const parsed = JSON.parse(raw) as CharacterRecord[];
      set({ characters: parsed });
    } catch {
      // Повреждённые/недоступные данные хранилища не должны ронять приложение —
      // остаёмся с пустым списком.
    }
  },
}));
