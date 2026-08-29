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

test('history frame seeds finished narration on reconnect', async () => {
  const { net, fake } = makeNet();
  await net.connect();
  fake.open();
  fake.push(sessionStateFrame({}));
  fake.push({ id: 5, chat_id: 'c', channel: 'chat', kind: 'data', op: 'history', payload: { type: 'narration', text: 'Готовая проза' } });
  expect(net.current().narration?.[0].text).toBe('Готовая проза');
  expect(net.current().narration?.[0].streaming).toBe(false);
});
