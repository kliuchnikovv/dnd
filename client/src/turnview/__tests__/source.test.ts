import { createHavenSource } from '../../mocks/haven/case';
import { freeIntent, tokenIntent } from '../intents';

describe('MockSource intent plumbing', () => {
    it('starts at the configured view', () => {
        const s = createHavenSource();
        expect(s.current().scene.node).toBe('harbor.tavern');
        expect(s.current().scene.title).toBe('Три Кита');
    });

    it('advances to the target view when a known option token is sent verbatim', async () => {
        const s = createHavenSource();
        const talk = s.current().options?.find((o) => o.label.includes('Берн'));
        expect(talk?.token).toBe('talk_bern');
        const next = await s.send(tokenIntent(talk!.token));
        // Диалог с Берном: реплика NPC.
        expect(next.narration?.[0]?.kind).toBe('npc');
        expect(next.narration?.[0]?.speaker?.name).toBe('Берн');
    });

    it('routes a check option to a resolution view', async () => {
        const s = createHavenSource('dialogueBern');
        await s.send(tokenIntent('persuade_bern'));
        expect(s.current().resolution?.outcome.tier).toBe('fail');
    });

    it('stays put on an unknown token (server would validate; mock does not move)', async () => {
        const s = createHavenSource();
        const before = s.current().scene.node;
        await s.send(tokenIntent('not_a_real_token'));
        expect(s.current().scene.node).toBe(before);
    });

    it('stays put on free text (mock has no generative answer)', async () => {
        const s = createHavenSource();
        const before = s.current().scene.title;
        await s.send(freeIntent('спросить про кассира'));
        expect(s.current().scene.title).toBe(before);
    });

    it('notifies subscribers on send', async () => {
        const s = createHavenSource();
        const seen: string[] = [];
        const off = s.subscribe((v) => seen.push(v.scene.title));
        await s.send(tokenIntent('open_map'));
        expect(seen).toContain('Пристань');
        off();
    });
});
