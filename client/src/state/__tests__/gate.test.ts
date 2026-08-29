import { screenFor } from '../gate';

describe('screenFor', () => {
  it('maps status to screen', () => {
    expect(screenFor('loading')).toBe('loading');
    expect(screenFor('signedOut')).toBe('signin');
    expect(screenFor('signedIn')).toBe('app');
  });
});
