import { NetSource, WebSocketLike, wsUrlFrom } from '../netSource';
import { TurnView } from '../../turnview/types';

class FakeSocket implements WebSocketLike {
  onopen: (() => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  sent: string[] = [];
  send(d: string) {
    this.sent.push(d);
  }
  close() {
    this.onclose?.();
  }
  push(frame: unknown) {
    this.onmessage?.({ data: JSON.stringify(frame) });
  }
  open() {
    this.onopen?.();
  }
}

function sessionStateFrame(tv: Partial<TurnView>) {
  return {
    id: 1,
    chat_id: 'c',
    channel: 'chat',
    kind: 'data',
    op: 'session_state',
    payload: {
      version: 1,
      scene: { node: 'n_quay', title: 'Причал' },
      options: [{ id: 'o1', label: 'Осмотреть', token: 'tok1' }],
      ...tv,
    },
  };
}

function makeNet() {
  const fake = new FakeSocket();
  let capturedUrl = '';
  const net = new NetSource({
    chatId: 'c',
    wsUrl: 'ws://x/chat/ws',
    getToken: async () => 'tok',
    onAuthLost: () => {},
    makeSocket: (url) => {
      capturedUrl = url;
      return fake;
    },
  });
  return { net, fake, getUrl: () => capturedUrl };
}

test('current() is a placeholder until session_state arrives', () => {
  const { net } = makeNet();
  expect(net.current().version).toBe(0);
});

test('session_state updates current() and notifies subscribers', async () => {
  const { net, fake } = makeNet();
  const seen: TurnView[] = [];
  net.subscribe((v) => seen.push(v));
  await net.connect();
  fake.open();
  fake.push(sessionStateFrame({}));
  expect(net.current().scene.title).toBe('Причал');
  expect(seen[seen.length - 1].options?.[0].token).toBe('tok1');
});

test('connect() builds the WS URL with token and chat_id', async () => {
  const { net, getUrl } = makeNet();
  await net.connect();
  expect(getUrl()).toContain('token=tok');
  expect(getUrl()).toContain('chat_id=c');
});

test('send maps token intent to a message frame and resolves on next session_state', async () => {
  const { net, fake } = makeNet();
  await net.connect();
  fake.open();
  fake.push(sessionStateFrame({}));
  const p = net.send({ kind: 'token', token: 'tok1' });
  const sent = JSON.parse(fake.sent[fake.sent.length - 1]);
  expect(sent.op).toBe('message');
  expect(sent.payload.token).toBe('tok1');
  fake.push(sessionStateFrame({ scene: { node: 'n_forge', title: 'Кузница' } }));
  const view = await p;
  expect(view.scene.title).toBe('Кузница');
});

test('free intent maps to text payload', async () => {
  const { net, fake } = makeNet();
  await net.connect();
  fake.open();
  fake.push(sessionStateFrame({}));
  const p = net.send({ kind: 'free', text: '1' });
  const sent = JSON.parse(fake.sent[fake.sent.length - 1]);
  expect(sent.payload.text).toBe('1');
  fake.push(sessionStateFrame({}));
  await p;
});

test('prose deltas accumulate into narration; done finalizes', async () => {
  const { net, fake } = makeNet();
  await net.connect();
  fake.open();
  fake.push(sessionStateFrame({}));
  fake.push({ id: 2, chat_id: 'c', channel: 'chat', kind: 'data', op: 'message', payload: { type: 'text', delta: 'Прич', ix: 0 } });
  fake.push({ id: 3, chat_id: 'c', channel: 'chat', kind: 'data', op: 'message', payload: { type: 'text', delta: 'ал в тумане', ix: 1 } });
  expect(net.current().narration?.[0].text).toBe('Причал в тумане');
  expect(net.current().narration?.[0].streaming).toBe(true);
  fake.push({ id: 4, chat_id: 'c', channel: 'chat', kind: 'meta', op: 'done' });
  expect(net.current().narration?.[0].streaming).toBe(false);
});

test('start op shows a pending (empty streaming) narration block; delta replaces it', async () => {
  const { net, fake } = makeNet();
  await net.connect();
  fake.open();
  fake.push(sessionStateFrame({}));
  fake.push({ id: 2, chat_id: 'c', channel: 'chat', kind: 'meta', op: 'start' });
  // Лоадер: один streaming-блок с пустым текстом.
  expect(net.current().narration?.length).toBe(1);
  expect(net.current().narration?.[0].text).toBe('');
  expect(net.current().narration?.[0].streaming).toBe(true);
  // Первая дельта заменяет лоадер текстом.
  fake.push({ id: 3, chat_id: 'c', channel: 'chat', kind: 'data', op: 'message', payload: { type: 'text', delta: 'Причал', ix: 0 } });
  expect(net.current().narration?.[0].text).toBe('Причал');
  expect(net.current().narration?.[0].streaming).toBe(true);
});

test('roll lands in the feed right after the action, in order', async () => {
  const { net, fake } = makeNet();
  await net.connect();
  fake.open();
  fake.push(sessionStateFrame({})); // options: tok1
  net.send({ kind: 'token', token: 'tok1' }); // эхо действия игрока
  // Ответ хода с броском.
  fake.push(
    sessionStateFrame({
      resolution: { target: 12, margin: 1, outcome: { label: 'с осложнением', tier: 'partial' }, terms: [{ label: 'внимание', value: 2 }] },
    }),
  );
  const n = net.current().narration ?? [];
  expect(n).toHaveLength(2);
  expect(n[0].kind).toBe('player');
  expect(n[1].kind).toBe('resolution');
  expect((n[1] as { resolution?: { target: number } }).resolution?.target).toBe(12);
});

test('no roll on a turn → no resolution block', async () => {
  const { net, fake } = makeNet();
  await net.connect();
  fake.open();
  fake.push(sessionStateFrame({}));
  net.send({ kind: 'token', token: 'tok1' });
  fake.push(sessionStateFrame({})); // без resolution
  const n = net.current().narration ?? [];
  expect(n.some((b) => b.kind === 'resolution')).toBe(false);
});

test('talk turn: gm framing then npc reply become two feed blocks', async () => {
  const { net, fake } = makeNet();
  await net.connect();
  fake.open();
  fake.push(sessionStateFrame({}));
  // Сегмент 1: обрамление Мастера.
  fake.push({ id: 2, chat_id: 'c', channel: 'chat', kind: 'meta', op: 'start', payload: { role: 'gm' } });
  fake.push({ id: 3, chat_id: 'c', channel: 'chat', kind: 'data', op: 'message', payload: { type: 'text', delta: 'Берн отводит взгляд.' } });
  // Сегмент 2: прямая речь NPC.
  fake.push({ id: 4, chat_id: 'c', channel: 'chat', kind: 'meta', op: 'start', payload: { role: 'npc', speaker: 'Берн' } });
  fake.push({ id: 5, chat_id: 'c', channel: 'chat', kind: 'data', op: 'message', payload: { type: 'text', delta: '— Не знаю никакого кассира.' } });
  fake.push({ id: 6, chat_id: 'c', channel: 'chat', kind: 'meta', op: 'done' });

  const n = net.current().narration ?? [];
  expect(n).toHaveLength(2);
  expect(n[0].kind).toBe('gm');
  expect(n[0].text).toBe('Берн отводит взгляд.');
  expect(n[1].kind).toBe('npc');
  expect(n[1].text).toBe('— Не знаю никакого кассира.');
  expect(n[1].speaker?.name).toBe('Берн');
  expect(n[1].streaming).toBeFalsy();
});

test('error frame clears a pending loader (no prose generated)', async () => {
  const { net, fake } = makeNet();
  await net.connect();
  fake.open();
  fake.push(sessionStateFrame({}));
  fake.push({ id: 2, chat_id: 'c', channel: 'chat', kind: 'meta', op: 'start' });
  expect(net.current().narration?.[0].streaming).toBe(true);
  fake.push({ id: 3, chat_id: 'c', channel: 'chat', kind: 'error', op: 'message', error: { message: 'сбой' } });
  expect(net.current().narration).toEqual([]);
});

test('reconnects after unexpected close and re-applies session_state', async () => {
  const sockets: FakeSocket[] = [];
  const net = new NetSource({
    chatId: 'c',
    wsUrl: 'ws://x/chat/ws',
    getToken: async () => 'tok',
    onAuthLost: () => {},
    makeSocket: () => {
      const f = new FakeSocket();
      sockets.push(f);
      return f;
    },
  });
  // без реальных таймеров: инжектируй немедленный планировщик
  (net as any).schedule = (fn: () => void) => fn();
  await net.connect();
  sockets[0].open();
  sockets[0].push(sessionStateFrame({}));
  sockets[0].onclose?.(); // неожиданный обрыв
  await Promise.resolve();
  expect(sockets.length).toBe(2); // переподключился
  sockets[1].open();
  sockets[1].push(sessionStateFrame({ scene: { node: 'n2', title: 'После' } }));
  expect(net.current().scene.title).toBe('После');
});

test('close() stops reconnect', async () => {
  const sockets: FakeSocket[] = [];
  const net = new NetSource({
    chatId: 'c',
    wsUrl: 'ws://x',
    getToken: async () => 'tok',
    onAuthLost: () => {},
    makeSocket: () => {
      const f = new FakeSocket();
      sockets.push(f);
      return f;
    },
  });
  (net as any).schedule = (fn: () => void) => fn();
  await net.connect();
  sockets[0].open();
  net.close();
  sockets[0].onclose?.();
  await Promise.resolve();
  expect(sockets.length).toBe(1); // нет реконнекта после close()
});

test('getToken() throws on connect → onAuthLost, closed, no socket, no reconnect scheduled', async () => {
  const onAuthLost = jest.fn();
  let scheduled = false;
  const net = new NetSource({
    chatId: 'c',
    wsUrl: 'ws://x',
    getToken: async () => {
      throw new Error('no token');
    },
    onAuthLost,
    makeSocket: () => {
      throw new Error('makeSocket should not be called');
    },
  });
  (net as any).schedule = () => {
    scheduled = true;
  };
  await net.connect();
  expect(onAuthLost).toHaveBeenCalledTimes(1);
  expect((net as any).closed).toBe(true);
  expect(scheduled).toBe(false);
});

test('ws.onerror (transient) with a valid token schedules a backoff reconnect, not onAuthLost', async () => {
  const sockets: FakeSocket[] = [];
  const onAuthLost = jest.fn();
  const captured: { fn: (() => void) | null } = { fn: null };
  const net = new NetSource({
    chatId: 'c',
    wsUrl: 'ws://x',
    getToken: async () => 'tok',
    onAuthLost,
    makeSocket: () => {
      const f = new FakeSocket();
      sockets.push(f);
      return f;
    },
  });
  (net as any).schedule = (fn: () => void) => {
    captured.fn = fn;
  };
  await net.connect();
  sockets[0].open();
  sockets[0].onerror?.(); // неожиданная сетевая ошибка, не auth

  expect(captured.fn).not.toBeNull(); // реконнект запланирован (backoff), не выполнен немедленно
  expect(onAuthLost).not.toHaveBeenCalled();
  expect(sockets.length).toBe(1); // ещё не переподключились — ждём таймер

  captured.fn?.();
  await Promise.resolve();
  expect(sockets.length).toBe(2); // теперь переподключились
  expect(onAuthLost).not.toHaveBeenCalled();
});

test('onerror followed by onclose on the same socket does not double-schedule a reconnect', async () => {
  const sockets: FakeSocket[] = [];
  let scheduleCalls = 0;
  const net = new NetSource({
    chatId: 'c',
    wsUrl: 'ws://x',
    getToken: async () => 'tok',
    onAuthLost: () => {},
    makeSocket: () => {
      const f = new FakeSocket();
      sockets.push(f);
      return f;
    },
  });
  (net as any).schedule = (fn: () => void) => {
    scheduleCalls += 1;
    fn();
  };
  await net.connect();
  sockets[0].open();
  sockets[0].onerror?.();
  sockets[0].onclose?.(); // тот же сокет — второй эвент не должен планировать ещё один реконнект
  await Promise.resolve();
  expect(scheduleCalls).toBe(1);
  expect(sockets.length).toBe(2);
});

test('close() cancels a pending (scheduled but not yet fired) reconnect', async () => {
  const sockets: FakeSocket[] = [];
  const captured: { fn: (() => void) | null } = { fn: null };
  const net = new NetSource({
    chatId: 'c',
    wsUrl: 'ws://x',
    getToken: async () => 'tok',
    onAuthLost: () => {},
    makeSocket: () => {
      const f = new FakeSocket();
      sockets.push(f);
      return f;
    },
  });
  // планировщик НЕ запускает fn сразу — ловим её, чтобы вручную дёрнуть после close()
  (net as any).schedule = (fn: () => void) => {
    captured.fn = fn;
  };
  await net.connect();
  sockets[0].open();
  sockets[0].onclose?.(); // неожиданный обрыв — реконнект запланирован, но ещё не выполнен
  expect(captured.fn).not.toBeNull();

  net.close(); // должен погасить запланированный реконнект
  captured.fn?.(); // теперь срабатывает таймер
  await Promise.resolve();
  await Promise.resolve();

  expect(sockets.length).toBe(1); // новый сокет не создан
});

test('token loss surfacing during a reconnect attempt: getToken throws → onAuthLost once, no further reconnect loop', async () => {
  const sockets: FakeSocket[] = [];
  const onAuthLost = jest.fn();
  let tokenLost = false;
  const net = new NetSource({
    chatId: 'c',
    wsUrl: 'ws://x',
    getToken: async () => {
      if (tokenLost) throw new Error('refresh failed, no token');
      return 'tok';
    },
    onAuthLost,
    makeSocket: () => {
      const f = new FakeSocket();
      sockets.push(f);
      return f;
    },
  });
  (net as any).schedule = (fn: () => void) => fn();
  await net.connect();
  sockets[0].open();

  tokenLost = true; // имитирует: refresh не удался, токены очищены, пока сокет ещё жил
  sockets[0].onclose?.(); // неожиданный обрыв → reconnect → connect() видит, что getToken бросает
  await Promise.resolve();
  await Promise.resolve();
  expect(onAuthLost).toHaveBeenCalledTimes(1);
  expect(sockets.length).toBe(1); // новый сокет не создан — connect() вернулся до makeSocket

  // дальнейшие события того же (уже отключённого) сокета не должны ничего запускать повторно
  sockets[0].onerror?.();
  await Promise.resolve();
  await Promise.resolve();
  expect(sockets.length).toBe(1);
  expect(onAuthLost).toHaveBeenCalledTimes(1);
});

test('transcript frame seeds the whole history on reconnect (player + gm)', async () => {
  const { net, fake } = makeNet();
  await net.connect();
  fake.open();
  fake.push(sessionStateFrame({}));
  fake.push({
    id: 5,
    chat_id: 'c',
    channel: 'chat',
    kind: 'data',
    op: 'transcript',
    payload: {
      type: 'transcript',
      entries: [
        { role: 'player', text: 'осмотреть журнал' },
        { role: 'gm', text: 'Готовая проза' },
      ],
    },
  });
  const n = net.current().narration ?? [];
  expect(n).toHaveLength(2);
  expect(n[0].kind).toBe('player');
  expect(n[0].text).toBe('осмотреть журнал');
  expect(n[1].kind).toBe('gm');
  expect(n[1].text).toBe('Готовая проза');
  expect(n[1].streaming).toBeFalsy();
});

test('optimistic echo: send pushes a player block into the feed', async () => {
  const { net, fake } = makeNet();
  await net.connect();
  fake.open();
  fake.push(sessionStateFrame({})); // options: [{label:'Осмотреть', token:'tok1'}]
  net.send({ kind: 'token', token: 'tok1' });
  const n = net.current().narration ?? [];
  expect(n[n.length - 1].kind).toBe('player');
  expect(n[n.length - 1].text).toBe('Осмотреть');
});

test('transcript is authoritative: reconnect overwrites optimistic echo', async () => {
  const { net, fake } = makeNet();
  await net.connect();
  fake.open();
  fake.push(sessionStateFrame({}));
  net.send({ kind: 'token', token: 'tok1' }); // оптимистичный player-блок
  fake.push({
    id: 9,
    chat_id: 'c',
    channel: 'chat',
    kind: 'data',
    op: 'transcript',
    payload: { type: 'transcript', entries: [{ role: 'player', text: 'осмотреть журнал у стойки' }] },
  });
  const n = net.current().narration ?? [];
  expect(n).toHaveLength(1);
  expect(n[0].text).toBe('осмотреть журнал у стойки');
});

test('wsUrlFrom converts http→ws and https→wss and appends /chat/ws', () => {
  expect(wsUrlFrom('http://localhost:8080')).toBe('ws://localhost:8080/chat/ws');
  expect(wsUrlFrom('https://api.example.com')).toBe('wss://api.example.com/chat/ws');
});
