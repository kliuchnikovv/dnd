// Frame — TS-зеркало server/frame.go. Единица обмена по WS-каналу хода.
export interface Frame {
  id: number;
  chat_id: string;
  channel: string;
  kind: string;
  op: string;
  payload?: any;
  error?: any;
}

export const Channel = { chat: 'chat', user: 'user' } as const;
export const Kind = { data: 'data', meta: 'meta', signal: 'signal', error: 'error' } as const;
export const Op = {
  message: 'message',
  ping: 'ping',
  start: 'start',
  done: 'done',
  history: 'history',
  transcript: 'transcript',
  sessionState: 'session_state',
  stop: 'stop',
} as const;
