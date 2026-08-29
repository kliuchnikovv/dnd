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
