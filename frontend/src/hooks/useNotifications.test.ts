import { describe, it, expect } from 'vitest';
import { shouldNotify } from './useNotifications';

const base = {
  channelKind: 2 as const, // 2 = DM
  channelId: '1',
  mentions: [] as string[],
};

describe('shouldNotify', () => {
  it('returns false when the actor is the current user', () => {
    expect(shouldNotify({ ...base, authorId: '42', currentUserId: '42' }, 'ALL')).toBe(false);
  });

  it('returns true for a message from another user on ALL', () => {
    expect(shouldNotify({ ...base, authorId: '7', currentUserId: '42' }, 'ALL')).toBe(true);
  });

  it('returns false for a muted channel regardless of author', () => {
    expect(shouldNotify({ ...base, authorId: '7', currentUserId: '42' }, 'MUTED')).toBe(false);
  });

  it('returns false on MENTIONS_ONLY when not mentioned', () => {
    expect(shouldNotify({ ...base, authorId: '7', currentUserId: '42' }, 'MENTIONS_ONLY')).toBe(false);
  });

  it('returns true on MENTIONS_ONLY when mentioned', () => {
    expect(shouldNotify({ ...base, authorId: '7', currentUserId: '42', mentions: ['42'] }, 'MENTIONS_ONLY')).toBe(true);
  });
});
