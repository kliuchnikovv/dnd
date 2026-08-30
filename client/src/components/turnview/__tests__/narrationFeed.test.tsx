import React from 'react';
import { render } from '@testing-library/react-native';

jest.mock('@react-native-async-storage/async-storage', () =>
    require('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);

import { ThemeProvider } from '../../../theme/ThemeContext';
import { NarrationFeed } from '../NarrationFeed';
import { Block } from '../../../turnview/types';

function mount(narration: Block[]) {
    return render(
        <ThemeProvider>
            <NarrationFeed narration={narration} />
        </ThemeProvider>,
    );
}

test('пустой streaming-блок рисует лоадер «Мастер печатает», не текст', async () => {
    const { getByLabelText } = await mount([{ kind: 'gm', text: '', streaming: true }]);
    expect(getByLabelText('Мастер печатает')).toBeTruthy();
});

test('streaming-блок с текстом рисует прозу, а не лоадер', async () => {
    const { getByText, queryByLabelText } = await mount([{ kind: 'gm', text: 'Причал в тумане', streaming: true }]);
    expect(getByText(/Причал в тумане/)).toBeTruthy();
    expect(queryByLabelText('Мастер печатает')).toBeNull();
});

test('player-блок рисует эхо действия под подписью ВЫ', async () => {
    const { getByText } = await mount([
        { kind: 'player', text: 'осмотреть журнал' },
        { kind: 'gm', text: 'В журнале не хватает страницы' },
    ]);
    expect(getByText('ВЫ')).toBeTruthy();
    expect(getByText('осмотреть журнал')).toBeTruthy();
    expect(getByText(/не хватает страницы/)).toBeTruthy();
});
