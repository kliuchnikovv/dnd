import React from 'react';
import { render, fireEvent, waitFor } from '@testing-library/react-native';

jest.mock('@react-native-async-storage/async-storage', () =>
    require('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);
jest.mock('expo-crypto', () => ({ randomUUID: jest.fn(() => 'uuid-1') }));
// react-native-safe-area-context в jsdom-окружении без реального layout не
// резолвит insets и держит дерево пустым (см. пакетный jest/mock.tsx) —
// подменяем на него, чтобы useSafeAreaInsets() внутри экрана сразу отдавал нули.
jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { ThemeProvider } from '../../theme/ThemeContext';
import { CharacterCreateScreen } from '../CharacterCreateScreen';
import { useCharacters } from '../../state/characters';

// Экран теперь трёхшаговый: правила → архетип → имя. Тест бьёт по всем трём
// переходам плюс по факту вызова useCharacters().add с итоговыми полями.
// SafeAreaProvider нужен из-за useSafeAreaInsets внутри экрана — без него
// react-native-safe-area-context падает с "No safe area value available".
// render() из @testing-library/react-native здесь асинхронный (как и в
// adventurePanel.test.tsx) — mount() нужно await'ить.
function mount() {
    return render(
        <SafeAreaProvider>
            <ThemeProvider>
                <CharacterCreateScreen />
            </ThemeProvider>
        </SafeAreaProvider>,
    );
}

describe('CharacterCreateScreen', () => {
    beforeEach(() => {
        useCharacters.setState({ characters: [] });
    });

    it('шаг 1: рендерит обе карточки ruleset — «Порог» и «D&D 5e»', async () => {
        const { getByText } = await mount();
        expect(getByText('Порог')).toBeTruthy();
        expect(getByText('D&D 5e')).toBeTruthy();
    });

    it('клик на «D&D 5e» показывает 4 dnd5e-архетипа', async () => {
        const { getByText } = await mount();
        await fireEvent.press(getByText('D&D 5e'));

        expect(getByText('Файтер')).toBeTruthy();
        expect(getByText('Рог')).toBeTruthy();
        expect(getByText('Рейнджер')).toBeTruthy();
        expect(getByText('Клерик')).toBeTruthy();
    });

    it('клик на «Порог» показывает threshold-архетипы', async () => {
        const { getByText } = await mount();
        await fireEvent.press(getByText('Порог'));

        expect(getByText('Инспектор')).toBeTruthy();
        expect(getByText('Пристав')).toBeTruthy();
        expect(getByText('Следопыт')).toBeTruthy();
        expect(getByText('Посредник')).toBeTruthy();
    });

    it('сохранение зовёт useCharacters().add с именем, ruleset и archetypeId', async () => {
        const addSpy = jest
            .fn()
            .mockResolvedValue({ id: 'chr_1', name: 'Аня', ruleset: 'dnd5e', archetypeId: 'rogue' });
        useCharacters.setState({ add: addSpy });

        const { getByText, getByPlaceholderText } = await mount();

        await fireEvent.press(getByText('D&D 5e'));
        await fireEvent.press(getByText('Рог'));
        await fireEvent.changeText(getByPlaceholderText('имя дознавателя'), 'Аня');
        await fireEvent.press(getByText('создать'));

        await waitFor(() => expect(addSpy).toHaveBeenCalledWith('Аня', 'dnd5e', 'rogue'));
    });
});
