import { useCallback, useEffect, useMemo, useState } from 'react';

import { VignetteNetSource, VignetteSnapshot } from '../net/vignetteNetSource';

// useVignetteView — аналог state/useTurnView.ts (см. тот файл), но привязан
// конкретно к VignetteNetSource, а не к абстрактному TurnViewSource: у
// виньетки нет альтернативного источника (MockSource) сегодня, заводить под
// неё интерфейс уровня TurnViewSource — преждевременная абстракция при одном
// реализующем классе. Если появится второй источник (фикстура для сторибука/
// тестов экрана) — тогда стоит вытащить VignetteSource-интерфейс, симметрично
// turnview/source.ts.
export function useVignetteView(source: VignetteNetSource) {
    const [snapshot, setSnapshot] = useState<VignetteSnapshot>(() => source.current());

    useEffect(() => {
        setSnapshot(source.current());
        const off = source.subscribe(setSnapshot);
        return off;
    }, [source]);

    const send = useCallback(
        (text: string) => {
            source.send(text).catch(() => {});
        },
        [source],
    );

    return useMemo(() => ({ snapshot, send }), [snapshot, send]);
}
