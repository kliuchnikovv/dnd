import { memoryTokenStore } from '../tokenStore';

describe('memoryTokenStore', () => {
  it('save → load round-trips, clear wipes', async () => {
    const s = memoryTokenStore();
    expect(await s.load()).toBeNull();
    await s.save({ accessToken: 'a', refreshToken: 'r', expiresAt: 123 });
    expect(await s.load()).toEqual({ accessToken: 'a', refreshToken: 'r', expiresAt: 123 });
    await s.clear();
    expect(await s.load()).toBeNull();
  });
});
