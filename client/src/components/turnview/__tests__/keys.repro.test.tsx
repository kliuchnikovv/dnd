import React from 'react';
import { render } from '@testing-library/react-native';

jest.mock('@react-native-async-storage/async-storage', () =>
    require('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);

import { ThemeProvider } from '../../../theme/ThemeContext';
import { OptionsRail } from '../OptionsRail';
import { Participants } from '../Participants';
import { listKey } from '../keys';
import { Option, Who } from '../../../turnview/types';

// Регресс: сервер прислал список с пустыми/повторяющимися id → React ронял
// «two children with the same key». Ключ должен оставаться уникальным всегда.
// (console.error спаить ненадёжно: React 19 кэширует ссылку до spy — поэтому
// проверяем инвариант ключа напрямую и убеждаемся, что рендер не падает.)

describe('listKey — уникальность независимо от id', () => {
    test('пустые id дают разные ключи', () => {
        const ids = ['', '', undefined, null];
        const keys = ids.map((id, i) => listKey(id, i));
        expect(new Set(keys).size).toBe(ids.length);
    });

    test('повторяющиеся непустые id дают разные ключи', () => {
        const ids = ['x', 'x', 'x'];
        const keys = ids.map((id, i) => listKey(id, i));
        expect(new Set(keys).size).toBe(ids.length);
    });
});

const noop = () => {};

test('OptionsRail: пустые option.id — рендерит все варианты', async () => {
    const options: Option[] = [
        { id: '', label: 'первый', token: 't1' },
        { id: '', label: 'второй', token: 't2' },
    ];
    const { getByText } = await render(
        <ThemeProvider>
            <OptionsRail options={options} onIntent={noop} />
        </ThemeProvider>,
    );
    expect(getByText('первый')).toBeTruthy();
    expect(getByText('второй')).toBeTruthy();
});

test('Participants: пустые who.id — рендерит всех NPC', async () => {
    const npcs: Who[] = [
        { id: '', name: 'Икс', disposition: 0 },
        { id: '', name: 'Игрек', disposition: 0 },
    ];
    const { getByText } = await render(
        <ThemeProvider>
            <Participants participants={npcs} />
        </ThemeProvider>,
    );
    expect(getByText('Икс')).toBeTruthy();
    expect(getByText('Игрек')).toBeTruthy();
});
