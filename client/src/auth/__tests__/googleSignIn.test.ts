import { fakeGoogleSignIn } from '../googleSignIn';

describe('fakeGoogleSignIn', () => {
  it('returns the id token', async () => {
    const g = fakeGoogleSignIn('id-abc');
    await expect(g.signIn()).resolves.toBe('id-abc');
  });
  it('throws when configured with an error', async () => {
    const g = fakeGoogleSignIn(new Error('cancelled'));
    await expect(g.signIn()).rejects.toThrow('cancelled');
  });
});
