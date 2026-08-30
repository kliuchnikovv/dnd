import React from 'react';
import { Pressable, StyleSheet, View } from 'react-native';
import Svg, { Circle, Line } from 'react-native-svg';

import { Text } from '@genie/ds';
import { useTheme } from '../../theme/ThemeContext';
import { MapView, MapNode } from '../../turnview/types';
import { Intent, tokenIntent } from '../../turnview/intents';
import { listKey } from './keys';

// MapGraph — узлы несут только то, что парти вправе знать (reachable/known из read-scope).
// Состояния: current (где мы), reachable (амбер, сплошная связь), known-недостижимый (пунктир,
// тускло), туман (known:false → мягкий «?»). Контракт не несёт «текущий» флаг — он приходит
// пропом currentNodeId. Тап по достижимому узлу шлёт переход по token (маппинг node→token — проп).

type NodeState = 'current' | 'reachable' | 'known' | 'fog';

function stateOf(n: MapNode, currentNodeId?: string): NodeState {
    if (n.id === currentNodeId) return 'current';
    if (n.reachable) return 'reachable';
    if (n.known) return 'known';
    return 'fog';
}

export const MapGraph: React.FC<{
    map?: MapView;
    currentNodeId?: string;
    onIntent: (intent: Intent) => void;
    /** node id → intent token перехода. Достижимый узел без токена — не кликается. */
    moveTokenFor?: (node: MapNode) => string | undefined;
    height?: number;
}> = ({ map, currentNodeId, onIntent, moveTokenFor, height = 320 }) => {
    const { descriptor: theme } = useTheme();
    const c = theme.colors;
    const nodes = map?.nodes ?? [];
    if (nodes.length === 0) return null;

    const current = nodes.find((n) => n.id === currentNodeId);
    const amber = c.accent;
    const dim = c.stroke ?? c.border;

    return (
        <View style={[styles.field, { height, backgroundColor: c.bgDeep ?? c.background, borderColor: c.hairline }]}>
            <Svg style={StyleSheet.absoluteFill} viewBox="0 0 100 100" preserveAspectRatio="none">
                {current
                    ? nodes.map((n, i) => {
                          if (n.id === current.id) return null;
                          const st = stateOf(n, currentNodeId);
                          if (st === 'fog') return null;
                          const reachable = st === 'reachable';
                          return (
                              <Line
                                  key={`l-${listKey(n.id, i)}`}
                                  x1={current.x}
                                  y1={current.y}
                                  x2={n.x}
                                  y2={n.y}
                                  stroke={reachable ? amber : dim}
                                  strokeWidth={reachable ? 0.6 : 0.4}
                                  strokeDasharray={reachable ? undefined : '2,2'}
                                  opacity={reachable ? 0.7 : 0.4}
                              />
                          );
                      })
                    : null}
                {nodes.map((n, i) => {
                    const st = stateOf(n, currentNodeId);
                    const r = st === 'current' ? 3.2 : st === 'reachable' ? 2.6 : 2.2;
                    const stroke = st === 'fog' ? dim : st === 'known' ? dim : amber;
                    return (
                        <Circle
                            key={`c-${listKey(n.id, i)}`}
                            cx={n.x}
                            cy={n.y}
                            r={r}
                            fill={c.surface}
                            stroke={stroke}
                            strokeWidth={st === 'current' ? 1 : 0.7}
                            strokeDasharray={st === 'known' ? '1.5,1.5' : undefined}
                            opacity={st === 'known' ? 0.55 : 1}
                        />
                    );
                })}
            </Svg>
            {/* Подписи и хит-таргеты поверх SVG (текст SVG-шрифтом ненадёжен в RN). */}
            {nodes.map((n, i) => {
                const st = stateOf(n, currentNodeId);
                const token = st === 'reachable' ? moveTokenFor?.(n) : undefined;
                const labelColor =
                    st === 'current' ? c.amberText : st === 'reachable' ? c.text : st === 'known' ? c.inkMuted : c.inkFaint;
                const content = (
                    <View style={styles.label} pointerEvents={token ? 'auto' : 'none'}>
                        <Text theme={theme} variant="mono" style={[styles.labelText, { color: labelColor }]}>
                            {st === 'fog' ? '?' : n.name}
                        </Text>
                        {st === 'current' ? (
                            <Text theme={theme} variant="mono" style={[styles.here, { color: c.amberText }]}>
                                ВЫ ЗДЕСЬ
                            </Text>
                        ) : null}
                    </View>
                );
                const style = [
                    styles.nodeOverlay,
                    { left: `${n.x}%` as const, top: `${n.y}%` as const },
                ];
                return token ? (
                    <Pressable key={`n-${listKey(n.id, i)}`} onPress={() => onIntent(tokenIntent(token))} hitSlop={12} style={style}>
                        {content}
                    </Pressable>
                ) : (
                    <View key={`n-${listKey(n.id, i)}`} style={style}>
                        {content}
                    </View>
                );
            })}
        </View>
    );
};

const styles = StyleSheet.create({
    field: { borderRadius: 16, borderWidth: 1, overflow: 'hidden' },
    nodeOverlay: {
        position: 'absolute',
        transform: [{ translateX: -40 }, { translateY: -8 }],
        width: 80,
        alignItems: 'center',
    },
    label: { alignItems: 'center', gap: 2, marginTop: 14 },
    labelText: { fontSize: 9.5, letterSpacing: 0.6 },
    here: { fontSize: 8, letterSpacing: 1.2 },
});
