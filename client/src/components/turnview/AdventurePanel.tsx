import React from 'react';
import { StyleSheet, View } from 'react-native';

import { Card, Text } from '@genie/ds';
import { useTheme } from '../../theme/ThemeContext';
import { Section, Slot } from '../../turnview/types';
import { listKey } from './keys';

// AdventurePanel — секции рабочей панели приключения от СЦЕНАРИЯ (kind "health"/"map"/
// "inventory"/"initiative" на Section.label — единственное поле, где core.PanelSection.Kind
// доезжает до клиента, см. core/scenario.go и core/scenarios/adventure/scenario.go). Каждый
// core.PanelSlot{Key,Value} ложится в Slot{name:Key,label:Value} без надстроек: клиент читает
// слот как готовую пару «что/значение», а не решает сам её форму. Panel.tsx делегирует сюда
// секции, чьи label входят в ADVENTURE_SECTION_KINDS; всё остальное (casebook-досье) остаётся
// на generic-рендере — эта секция его не касается.

export const ADVENTURE_SECTION_KINDS = new Set(['health', 'map', 'inventory', 'initiative']);

export const AdventureSectionView: React.FC<{ section: Section }> = ({ section }) => {
    switch (section.label) {
        case 'health':
            return <HealthSection slots={section.slots ?? []} />;
        case 'map':
            return <MapSection slots={section.slots ?? []} />;
        case 'inventory':
            return <InventorySection slots={section.slots ?? []} />;
        case 'initiative':
            return <InitiativeSection slots={section.slots ?? []} />;
        default:
            return null;
    }
};

function findSlot(slots: Slot[], name: string): Slot | undefined {
    return slots.find((s) => s.name === name);
}

// parseHP — «N/M» без словаря конкретного правила: клиент не знает, что это HP правила
// D&D 5e, только что это дробь «текущее/предел» пришедшая готовой строкой.
function parseHP(value: string | undefined): { current: number; max: number } | undefined {
    if (!value) return undefined;
    const m = /^(\d+)\s*\/\s*(\d+)$/.exec(value.trim());
    if (!m) return undefined;
    return { current: Number(m[1]), max: Number(m[2]) };
}

const HealthSection: React.FC<{ slots: Slot[] }> = ({ slots }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const hp = findSlot(slots, 'hp');
    const parsed = parseHP(hp?.label);

    return (
        <Card theme={theme} surface="surface" radius="lg" bordered style={[styles.card, { borderColor: c.hairline }]}>
            <Text theme={theme} variant="mono" style={[styles.cardLabel, { color: c.inkMuted }]}>
                ЗДОРОВЬЕ
            </Text>
            {parsed ? (
                <>
                    <View style={[styles.barTrack, { backgroundColor: c.stroke ?? c.border }]}>
                        <View
                            style={[
                                styles.barFill,
                                {
                                    width: `${Math.max(0, Math.min(1, parsed.current / Math.max(1, parsed.max))) * 100}%`,
                                    backgroundColor: c.accent,
                                },
                            ]}
                        />
                    </View>
                    <Text theme={theme} variant="body" style={{ color: c.text }}>
                        {hp!.label}
                    </Text>
                </>
            ) : (
                <Text theme={theme} variant="body" style={{ color: c.text }}>
                    {hp?.label ?? '—'}
                </Text>
            )}
        </Card>
    );
};

const MapSection: React.FC<{ slots: Slot[] }> = ({ slots }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const here = findSlot(slots, 'here');
    const reachable = slots.filter((s) => s.name !== 'here');

    return (
        <Card theme={theme} surface="surface" radius="lg" bordered style={[styles.card, { borderColor: c.hairline }]}>
            <Text theme={theme} variant="mono" style={[styles.cardLabel, { color: c.inkMuted }]}>
                КАРТА
            </Text>
            {here ? (
                <Text theme={theme} variant="body" style={{ color: c.text }}>
                    Здесь: {here.label}
                </Text>
            ) : null}
            {reachable.length > 0 ? (
                <View style={styles.list}>
                    {reachable.map((s, i) => (
                        <Text key={listKey(s.name, i)} theme={theme} variant="body" style={[styles.listRow, { color: c.inkFaint }]}>
                            {s.label}
                        </Text>
                    ))}
                </View>
            ) : null}
        </Card>
    );
};

const InventorySection: React.FC<{ slots: Slot[] }> = ({ slots }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;

    return (
        <Card theme={theme} surface="surface" radius="lg" bordered style={[styles.card, { borderColor: c.hairline }]}>
            <Text theme={theme} variant="mono" style={[styles.cardLabel, { color: c.inkMuted }]}>
                ИНВЕНТАРЬ
            </Text>
            {slots.length > 0 ? (
                <View style={styles.list}>
                    {slots.map((s, i) => (
                        <Text key={listKey(s.name, i)} theme={theme} variant="body" style={[styles.listRow, { color: c.text }]}>
                            {s.label}
                        </Text>
                    ))}
                </View>
            ) : (
                <Text theme={theme} variant="body" style={{ color: c.inkFaint }}>
                    пусто
                </Text>
            )}
        </Card>
    );
};

const InitiativeSection: React.FC<{ slots: Slot[] }> = ({ slots }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const round = findSlot(slots, 'round');
    const current = findSlot(slots, 'current');
    const action = findSlot(slots, 'action');
    const bonus = findSlot(slots, 'bonus');

    return (
        <Card theme={theme} surface="surface" radius="lg" bordered style={[styles.card, { borderColor: c.hairline }]}>
            <Text theme={theme} variant="mono" style={[styles.cardLabel, { color: c.inkMuted }]}>
                ИНИЦИАТИВА
            </Text>
            {round ? (
                <Text theme={theme} variant="body" style={{ color: c.text }}>
                    Раунд {round.label}
                </Text>
            ) : null}
            {current ? (
                <Text theme={theme} variant="body" style={{ color: c.text }}>
                    Ход: {current.label}
                </Text>
            ) : null}
            <View style={styles.flagRow}>
                {action ? <FlagBadge label="действие" value={action.label} /> : null}
                {bonus ? <FlagBadge label="бонусное" value={bonus.label} /> : null}
            </View>
        </Card>
    );
};

const FlagBadge: React.FC<{ label: string; value: string }> = ({ label, value }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const available = /^(y|yes|true|1)$/i.test(value.trim());
    return (
        <View
            style={[
                styles.flag,
                { borderColor: available ? c.accent : (c.stroke ?? c.border), opacity: available ? 1 : 0.5 },
            ]}
        >
            <Text theme={theme} variant="caption" style={{ color: available ? c.accent : c.inkFaint }}>
                {label}
            </Text>
        </View>
    );
};

const styles = StyleSheet.create({
    card: { gap: 6 },
    cardLabel: { fontSize: 9.5, letterSpacing: 1.4 },
    barTrack: { height: 6, borderRadius: 999, overflow: 'hidden' },
    barFill: { height: 6, borderRadius: 999 },
    list: { gap: 2 },
    listRow: { fontSize: 13 },
    flagRow: { flexDirection: 'row', gap: 8 },
    flag: { borderWidth: 1, borderRadius: 999, paddingVertical: 3, paddingHorizontal: 8 },
});
