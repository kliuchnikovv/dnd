import * as SecureStore from 'expo-secure-store';

export type StoredTokens = { accessToken: string; refreshToken: string; expiresAt: number };

export interface TokenStore {
  load(): Promise<StoredTokens | null>;
  save(t: StoredTokens): Promise<void>;
  clear(): Promise<void>;
}

const KEY = 'dnd.auth.tokens';

// secureTokenStore — прод-хранилище: токены в защищённом хранилище устройства,
// не в AsyncStorage. Одним ключом, JSON.
export const secureTokenStore: TokenStore = {
  async load() {
    const raw = await SecureStore.getItemAsync(KEY);
    if (!raw) return null;
    try {
      return JSON.parse(raw) as StoredTokens;
    } catch {
      return null;
    }
  },
  async save(t) {
    await SecureStore.setItemAsync(KEY, JSON.stringify(t));
  },
  async clear() {
    await SecureStore.deleteItemAsync(KEY);
  },
};

// memoryTokenStore — для тестов и веб-превью, где secure-store недоступен.
export function memoryTokenStore(): TokenStore {
  let cur: StoredTokens | null = null;
  return {
    async load() {
      return cur;
    },
    async save(t) {
      cur = t;
    },
    async clear() {
      cur = null;
    },
  };
}
