import { useCallback, useEffect, useMemo, useState } from 'react';

import { TurnViewSource } from '../turnview/source';
import { TurnView } from '../turnview/types';
import { Intent } from '../turnview/intents';

// useTurnView — тонкая привязка источника вида к состоянию экрана. Экран не считает ничего:
// он показывает current() и шлёт интенты через send(). Сегодня источник — MockSource,
// завтра NetSource; хук не меняется.
export function useTurnView(source: TurnViewSource) {
    const [view, setView] = useState<TurnView>(() => source.current());

    useEffect(() => {
        setView(source.current());
        const off = source.subscribe(setView);
        return off;
    }, [source]);

    const send = useCallback(
        (intent: Intent) => {
            source.send(intent).catch(() => {});
        },
        [source],
    );

    return useMemo(() => ({ view, send }), [view, send]);
}
