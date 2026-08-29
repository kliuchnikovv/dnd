import { NetSource, WebSocketLike } from '../netSource';
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

function errorFrame(code: number) {
  return {
    id: 1,
    chat_id: 'c',
    channel: 'chat',
    kind: 'error',
    op: 'message',
    error: { code },
  };
}

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

test('auth failure: 401 error frame retries getToken once, then reconnects; second 401 calls onAuthLost', async () => {
  const sockets: FakeSocket[] = [];
  const getToken = jest.fn(async () => 'tok');
  const onAuthLost = jest.fn();
  const net = new NetSource({
    chatId: 'c',
    wsUrl: 'ws://x',
    getToken,
    onAuthLost,
    makeSocket: () => {
      const f = new FakeSocket();
      sockets.push(f);
      return f;
    },
  });
  (net as any).schedule = (fn: () => void) => fn();
  await net.connect();
  expect(getToken).toHaveBeenCalledTimes(1);

  sockets[0].push(errorFrame(401));
  await Promise.resolve();
  await Promise.resolve();
  expect(getToken).toHaveBeenCalledTimes(2); // один рефреш через getToken
  expect(sockets.length).toBe(2); // реконнект
  expect(onAuthLost).not.toHaveBeenCalled();

  sockets[1].push(errorFrame(401));
  await Promise.resolve();
  await Promise.resolve();
  expect(onAuthLost).toHaveBeenCalledTimes(1); // повторный отказ — сдаёмся
  expect(sockets.length).toBe(2); // реконнект больше не пробуем
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

test('after giving up (second consecutive 401 → onAuthLost), no further reconnect loop', async () => {
  const sockets: FakeSocket[] = [];
  const onAuthLost = jest.fn();
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
  (net as any).schedule = (fn: () => void) => fn();
  await net.connect();

  sockets[0].push(errorFrame(401)); // первый отказ — рефреш и реконнект
  await Promise.resolve();
  await Promise.resolve();
  expect(sockets.length).toBe(2);

  sockets[1].push(errorFrame(401)); // второй подряд — сдаёмся
  await Promise.resolve();
  await Promise.resolve();
  expect(onAuthLost).toHaveBeenCalledTimes(1);
  expect(sockets.length).toBe(2);

  // сервер (или наш собственный close() внутри handleAuthFailure) закрывает сокет —
  // это НЕ должно снова уйти в backoff-реконнект и снова дёрнуть onAuthLost
  sockets[1].onclose?.();
  await Promise.resolve();
  await Promise.resolve();
  expect(sockets.length).toBe(2); // нет нового сокета
  expect(onAuthLost).toHaveBeenCalledTimes(1); // не вызван повторно
});

test('history frame seeds finished narration on reconnect', async () => {
  const { net, fake } = makeNet();
  await net.connect();
  fake.open();
  fake.push(sessionStateFrame({}));
  fake.push({ id: 5, chat_id: 'c', channel: 'chat', kind: 'data', op: 'history', payload: { type: 'narration', text: 'Готовая проза' } });
  expect(net.current().narration?.[0].text).toBe('Готовая проза');
  expect(net.current().narration?.[0].streaming).toBe(false);
});
