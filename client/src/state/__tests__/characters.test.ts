jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);

let mockUuidCounter = 0;
jest.mock('expo-crypto', () => ({
  randomUUID: jest.fn(() => `uuid-${++mockUuidCounter}`),
}));

import AsyncStorage from '@react-native-async-storage/async-storage';
import { useCharacters } from '../characters';

describe('useCharacters', () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    useCharacters.setState({ characters: [] });
    mockUuidCounter = 0;
  });

  it('add: создаёт персонажа, добавляет в стор и сохраняет в AsyncStorage', async () => {
    const character = await useCharacters.getState().add('Аня', 'threshold', 'a.inspector');
    expect(character).toMatchObject({ name: 'Аня', ruleset: 'threshold', archetypeId: 'a.inspector' });
    expect(character.id).toBeTruthy();
    expect(useCharacters.getState().characters).toHaveLength(1);

    const raw = await AsyncStorage.getItem('dnd.characters.v1');
    expect(JSON.parse(raw as string)).toEqual(useCharacters.getState().characters);
  });

  it('getById: находит по id, undefined для неизвестного', async () => {
    const character = await useCharacters.getState().add('Борис', 'dnd5e', 'fighter');
    expect(useCharacters.getState().getById(character.id)).toEqual(character);
    expect(useCharacters.getState().getById('missing')).toBeUndefined();
  });

  it('hydrate: восстанавливает список из AsyncStorage после рестарта', async () => {
    await useCharacters.getState().add('Виктор', 'threshold', 'a.tracker');

    // Эмулируем перезапуск приложения: сбрасываем in-memory стор, но не хранилище.
    useCharacters.setState({ characters: [] });
    expect(useCharacters.getState().characters).toHaveLength(0);

    await useCharacters.getState().hydrate();
    expect(useCharacters.getState().characters).toHaveLength(1);
    expect(useCharacters.getState().characters[0]).toMatchObject({ name: 'Виктор', ruleset: 'threshold' });
  });

  it('hydrate: пустое хранилище — характеры остаются пустым списком', async () => {
    await useCharacters.getState().hydrate();
    expect(useCharacters.getState().characters).toEqual([]);
  });
});
