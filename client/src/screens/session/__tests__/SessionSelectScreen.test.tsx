import React from 'react';
import { render, fireEvent, waitFor } from '@testing-library/react-native';

jest.mock('@react-native-async-storage/async-storage', () =>
    require('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);
// react-native-safe-area-context в jsdom-окружении без реального layout не
// резолвит insets (см. пакетный jest/mock.tsx) — подменяем на него.
jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);

jest.mock('../../../state/useAuth', () => ({ useAuth: () => ({ status: 'signedIn' }) }));
jest.mock('../../../auth/tokenStore', () => ({
    secureTokenStore: { load: jest.fn().mockResolvedValue({ accessToken: 'tok', refreshToken: 'r', expiresAt: 0 }) },
}));

const mockListSessions = jest.fn().mockResolvedValue([]);
const mockCreateSession = jest.fn().mockResolvedValue({ chatId: 'chat_1' });
jest.mock('../../../net/sessionsClient', () => ({
    listSessions: (...args: unknown[]) => mockListSessions(...args),
    createSession: (...args: unknown[]) => mockCreateSession(...args),
}));

const mockListCases = jest.fn().mockResolvedValue([
    { id: 'harbour', name: 'Гавань', kind: 'adventure', rules: 'threshold', scenario: 'investigation', blurb: 'Дело о контрабанде' },
    { id: 'lighthouse', name: 'Маяк', kind: 'adventure', rules: 'dnd5e', scenario: 'adventure', blurb: 'Приключение у маяка' },
    { id: 'nightguest', name: 'Гость к ночи', kind: 'vignette', rules: 'vignette', scenario: 'vignette', blurb: 'Ночная сцена' },
]);
jest.mock('../../../net/cases', () => ({
    listCases: (...args: unknown[]) => mockListCases(...args),
}));

import { SafeAreaProvider } from 'react-native-safe-area-context';
import { ThemeProvider } from '../../../theme/ThemeContext';
import { SessionSelectScreen } from '../SessionSelectScreen';
import { useCharacters } from '../../../state/characters';

// Экран теперь начинает новое дело двухшагово: персонаж → совместимое дело.
// Тест бьёт по фильтрации (dnd5e-персонаж видит только dnd5e-дела) и по
// финальному вызову createSession(token, caseId, characterId).
function mount(onPick = jest.fn()) {
    return render(
        <SafeAreaProvider>
            <ThemeProvider>
                <SessionSelectScreen onPick={onPick} />
            </ThemeProvider>
        </SafeAreaProvider>,
    );
}

describe('SessionSelectScreen', () => {
    beforeEach(() => {
        jest.clearAllMocks();
        mockListSessions.mockResolvedValue([]);
        mockCreateSession.mockResolvedValue({ chatId: 'chat_1' });
        mockListCases.mockResolvedValue([
            { id: 'harbour', name: 'Гавань', rules: 'threshold', scenario: 'investigation', blurb: 'Дело о контрабанде' },
            { id: 'lighthouse', name: 'Маяк', rules: 'dnd5e', scenario: 'adventure', blurb: 'Приключение у маяка' },
        ]);
        useCharacters.setState({
            characters: [
                { id: 'chr_threshold', name: 'Аня', ruleset: 'threshold', archetypeId: 'a.inspector' },
                { id: 'chr_dnd5e', name: 'Борис', ruleset: 'dnd5e', archetypeId: 'rogue' },
            ],
        });
    });

    it('рендерит персонажей из store', async () => {
        const { getByText } = await mount();
        await waitFor(() => expect(getByText('Аня')).toBeTruthy());
        expect(getByText('Борис')).toBeTruthy();
    });

    it('выбор dnd5e-персонажа показывает только dnd5e-совместимые дела', async () => {
        const { getByText, queryByText } = await mount();
        await waitFor(() => expect(getByText('Борис')).toBeTruthy());

        await fireEvent.press(getByText('Борис'));

        await waitFor(() => expect(getByText('Маяк')).toBeTruthy());
        expect(queryByText('Гавань')).toBeNull();
    });

    it('«Начать» зовёт createSession(token, caseId, characterId) и onPick(chatId)', async () => {
        const onPick = jest.fn();
        const { getByText } = await mount(onPick);
        await waitFor(() => expect(getByText('Аня')).toBeTruthy());

        await fireEvent.press(getByText('Аня'));
        await waitFor(() => expect(getByText('Гавань')).toBeTruthy());
        await fireEvent.press(getByText('Гавань'));
        await fireEvent.press(getByText('Начать'));

        await waitFor(() => expect(mockCreateSession).toHaveBeenCalledWith('tok', 'harbour', 'chr_threshold'));
        expect(onPick).toHaveBeenCalledWith({ chatId: 'chat_1', kind: 'turn' });
    });

    it('vignette-персонаж видит виньеточное дело, onPick получает kind=vignette', async () => {
        mockListCases.mockResolvedValue([
            { id: 'harbour', name: 'Гавань', kind: 'adventure', rules: 'threshold', scenario: 'investigation', blurb: 'Дело о контрабанде' },
            { id: 'lighthouse', name: 'Маяк', kind: 'adventure', rules: 'dnd5e', scenario: 'adventure', blurb: 'Приключение у маяка' },
            { id: 'nightguest', name: 'Гость к ночи', kind: 'vignette', rules: 'vignette', scenario: 'vignette', blurb: 'Ночная сцена' },
        ]);
        useCharacters.setState({
            characters: [
                { id: 'chr_vig', name: 'Тень', ruleset: 'vignette', archetypeId: 'a.witness' },
            ],
        });
        const onPick = jest.fn();
        const { getByText, queryByText } = await mount(onPick);
        await waitFor(() => expect(getByText('Тень')).toBeTruthy());

        await fireEvent.press(getByText('Тень'));
        await waitFor(() => expect(getByText('Гость к ночи')).toBeTruthy());
        // Не должен показывать несовместимые.
        expect(queryByText('Гавань')).toBeNull();
        expect(queryByText('Маяк')).toBeNull();
        await fireEvent.press(getByText('Гость к ночи'));
        await fireEvent.press(getByText('Начать'));

        await waitFor(() => expect(mockCreateSession).toHaveBeenCalledWith('tok', 'nightguest', 'chr_vig'));
        expect(onPick).toHaveBeenCalledWith({ chatId: 'chat_1', kind: 'vignette' });
    });
});
