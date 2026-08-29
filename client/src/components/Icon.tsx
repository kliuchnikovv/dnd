import React from 'react';
import Svg, { Circle, Line, Path, Polyline } from 'react-native-svg';

// Линейные иконки в одном стиле: stroke 1.6, currentColor. Не эмодзи (правило контента).
export type IconName =
    | 'home'
    | 'globe'
    | 'magnifier'
    | 'user'
    | 'dots'
    | 'scale'
    | 'fire'
    | 'anchor'
    | 'back'
    | 'document'
    | 'gear';

export const Icon: React.FC<{ name: IconName; size?: number; color: string; strokeWidth?: number }> = ({
    name,
    size = 21,
    color,
    strokeWidth = 1.6,
}) => {
    const p = { stroke: color, strokeWidth, fill: 'none', strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const };
    return (
        <Svg width={size} height={size} viewBox="0 0 24 24">
            {name === 'home' ? (
                <>
                    <Path d="M4 11l8-6 8 6" {...p} />
                    <Path d="M6 10v9h12v-9" {...p} />
                </>
            ) : null}
            {name === 'globe' ? (
                <>
                    <Circle cx={12} cy={12} r={8} {...p} />
                    <Path d="M4 12h16M12 4c2.5 2.5 2.5 13 0 16M12 4c-2.5 2.5-2.5 13 0 16" {...p} />
                </>
            ) : null}
            {name === 'magnifier' ? (
                <>
                    <Circle cx={11} cy={11} r={6} {...p} />
                    <Line x1={16} y1={16} x2={20} y2={20} {...p} />
                </>
            ) : null}
            {name === 'user' ? (
                <>
                    <Circle cx={12} cy={8} r={4} {...p} />
                    <Path d="M5 20c0-3.5 3-6 7-6s7 2.5 7 6" {...p} />
                </>
            ) : null}
            {name === 'dots' ? (
                <>
                    <Circle cx={5} cy={12} r={1.4} fill={color} stroke="none" />
                    <Circle cx={12} cy={12} r={1.4} fill={color} stroke="none" />
                    <Circle cx={19} cy={12} r={1.4} fill={color} stroke="none" />
                </>
            ) : null}
            {name === 'scale' ? (
                <>
                    <Line x1={12} y1={4} x2={12} y2={20} {...p} />
                    <Path d="M5 8h14M5 8l-2 5h4zM19 8l-2 5h4z" {...p} />
                    <Line x1={8} y1={20} x2={16} y2={20} {...p} />
                </>
            ) : null}
            {name === 'fire' ? (
                <Path d="M12 3c1 3-2 4-2 7a2 2 0 004 0c0-1 0-1 1-2 1 2 2 3 2 6a5 5 0 01-10 0c0-4 4-6 5-11z" {...p} />
            ) : null}
            {name === 'anchor' ? (
                <>
                    <Circle cx={12} cy={5} r={2} {...p} />
                    <Line x1={12} y1={7} x2={12} y2={20} {...p} />
                    <Path d="M5 12a7 7 0 0014 0" {...p} />
                    <Line x1={5} y1={12} x2={8} y2={12} {...p} />
                    <Line x1={19} y1={12} x2={16} y2={12} {...p} />
                </>
            ) : null}
            {name === 'back' ? <Polyline points="15 5 8 12 15 19" {...p} /> : null}
            {name === 'document' ? (
                <>
                    <Path d="M7 3h7l4 4v14H7z" {...p} />
                    <Polyline points="14 3 14 7 18 7" {...p} />
                    <Line x1={9.5} y1={12} x2={15} y2={12} {...p} />
                    <Line x1={9.5} y1={15} x2={15} y2={15} {...p} />
                </>
            ) : null}
            {name === 'gear' ? (
                <>
                    <Circle cx={12} cy={12} r={3} {...p} />
                    <Path d="M12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M19 5l-2 2M7 17l-2 2" {...p} />
                </>
            ) : null}
        </Svg>
    );
};
