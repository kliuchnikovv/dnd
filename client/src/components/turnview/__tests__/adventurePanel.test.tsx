import React from 'react';
import { render } from '@testing-library/react-native';

jest.mock('@react-native-async-storage/async-storage', () =>
    require('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);

import { ThemeProvider } from '../../../theme/ThemeContext';
import { Panel } from '../Panel';
import { Panel as PanelData } from '../../../turnview/types';

// Панель приключения — вымышленный ход из core/scenarios/adventure/scenario.go: health/map/
// inventory/initiative приходят как core.PanelSection{Kind,Slots:[{Key,Value}]}, на клиенте это
// Section.label = Kind, Slot{name:Key,label:Value}. Тест бьёт по всем четырём секциям разом,
// чтобы поймать регресс роутинга «по kind» из Panel.tsx.
const adventurePanel: PanelData = {
    kind: 'adventure',
    title: 'Приключение',
    surface: true,
    sections: [
        { label: 'health', slots: [{ name: 'hp', label: '8/12' }] },
        {
            label: 'map',
            slots: [
                { name: 'here', label: 'Таверна «Ржавый якорь»' },
                { name: 'n_dock', label: 'Причал' },
                { name: 'n_road', label: 'Северная дорога' },
            ],
        },
        {
            label: 'inventory',
            slots: [
                { name: 'sword01', label: 'Короткий меч' },
                { name: 'torch01', label: 'Факел' },
            ],
        },
        {
            label: 'initiative',
            slots: [
                { name: 'round', label: '2' },
                { name: 'current', label: 'Гоблин-разведчик' },
                { name: 'action', label: 'true' },
                { name: 'bonus', label: 'false' },
            ],
        },
    ],
};

function mount(panel: PanelData) {
    return render(
        <ThemeProvider>
            <Panel panel={panel} onIntent={() => {}} />
        </ThemeProvider>,
    );
}

test('панель приключения рисует все четыре секции: здоровье, карту, инвентарь, инициативу', async () => {
    const { getByText } = await mount(adventurePanel);

    expect(getByText('ЗДОРОВЬЕ')).toBeTruthy();
    expect(getByText('8/12')).toBeTruthy();

    expect(getByText('КАРТА')).toBeTruthy();
    expect(getByText(/Таверна «Ржавый якорь»/)).toBeTruthy();
    expect(getByText('Причал')).toBeTruthy();
    expect(getByText('Северная дорога')).toBeTruthy();

    expect(getByText('ИНВЕНТАРЬ')).toBeTruthy();
    expect(getByText('Короткий меч')).toBeTruthy();
    expect(getByText('Факел')).toBeTruthy();

    expect(getByText('ИНИЦИАТИВА')).toBeTruthy();
    expect(getByText(/Раунд 2/)).toBeTruthy();
    expect(getByText(/Гоблин-разведчик/)).toBeTruthy();
    expect(getByText('действие')).toBeTruthy();
    expect(getByText('бонусное')).toBeTruthy();
});

test('панель без секций-архетипа (casebook) рисуется generic-путём без падения', async () => {
    const casebook: PanelData = {
        kind: 'deduction',
        title: 'Досье',
        surface: true,
        sections: [{ label: 'Факты', items: [{ id: 'f1', label: 'Отпечаток на двери' }] }],
    };
    const { getByText, queryByText } = await mount(casebook);
    expect(getByText('Отпечаток на двери')).toBeTruthy();
    expect(queryByText('ЗДОРОВЬЕ')).toBeNull();
});
