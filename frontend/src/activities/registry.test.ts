import { describe, it, expect, beforeEach } from 'vitest';
import { register, lookup, list, _resetForTests } from './registry';

describe('activity registry', () => {
  beforeEach(() => _resetForTests());

  it('lookup returns null for unregistered types', () => {
    expect(lookup('nope')).toBeNull();
  });

  it('register stores a definition', () => {
    const def = { type: 'foo', displayName: 'Foo', render: () => null };
    register(def);
    expect(lookup('foo')).toBe(def);
    expect(list()).toContain(def);
  });

  it('register replaces by type', () => {
    register({ type: 'foo', displayName: 'A', render: () => null });
    register({ type: 'foo', displayName: 'B', render: () => null });
    expect(lookup('foo')!.displayName).toBe('B');
    expect(list()).toHaveLength(1);
  });
});
