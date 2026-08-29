import React from 'react';

import { useTurnView } from '../state/useTurnView';
import { useAppStore, sessionModeOf } from '../state/store';
import { tokenIntent } from '../turnview/intents';
import { SceneScreen } from './SceneScreen';
import { DialogueScreen } from './DialogueScreen';
import { MapScreen } from './MapScreen';
import { DossierScreen } from './DossierScreen';

// Сессия (таб «Дело») — один поток на общем источнике. Какой под-экран показать, решают
// ДАННЫЕ вида (sessionModeOf), а не интерпретация токена: карта пришла — карта, панель всплыла —
// досье, игрок в разговоре — диалог, иначе сцена.
export const SessionScreen: React.FC = () => {
    const source = useAppStore((s) => s.source);
    const { view, send } = useTurnView(source);
    const mode = sessionModeOf(view);

    switch (mode) {
        case 'dialogue':
            return <DialogueScreen view={view} onIntent={send} onExit={() => send(tokenIntent('back_scene'))} />;
        case 'map':
            return <MapScreen view={view} onIntent={send} />;
        case 'dossier':
            return <DossierScreen view={view} onIntent={send} />;
        case 'scene':
        default:
            return <SceneScreen view={view} onIntent={send} />;
    }
};
