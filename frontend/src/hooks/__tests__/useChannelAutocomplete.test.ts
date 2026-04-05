import { describe, it, expect } from 'vitest';
import { detectChannelTag, insertChannelTag, resolveChannelTags } from '../useChannelAutocomplete';
import type { Channel } from '../../api/types';

function makeChannel(overrides: Partial<Channel> = {}): Channel {
  return {
    id: '1',
    server_id: 's1',
    name: 'general',
    type: 0,
    position: 0,
    created_at: '',
    updated_at: '',
    ...overrides,
  };
}

describe('detectChannelTag', () => {
  it('returns null when no # is present', () => {
    expect(detectChannelTag('hello world', 5)).toBeNull();
  });

  it('detects a tag at end of text', () => {
    expect(detectChannelTag('hey #gen', 8)).toEqual({
      query: 'gen',
      start: 4,
      end: 8,
    });
  });

  it('detects a tag with empty query right after #', () => {
    expect(detectChannelTag('hey #', 5)).toEqual({
      query: '',
      start: 4,
      end: 5,
    });
  });

  it('returns null when whitespace follows #', () => {
    expect(detectChannelTag('hey # gen', 9)).toBeNull();
  });

  it('returns null when cursor is before #', () => {
    expect(detectChannelTag('hey #gen', 3)).toBeNull();
  });

  it('uses the last # when multiple exist', () => {
    expect(detectChannelTag('a #foo b #bar', 13)).toEqual({
      query: 'bar',
      start: 9,
      end: 13,
    });
  });

  it('detects tag when # is at position 0', () => {
    expect(detectChannelTag('#general', 8)).toEqual({
      query: 'general',
      start: 0,
      end: 8,
    });
  });

  it('returns null when cursor is right on #', () => {
    // cursor at hashIdx means slice(hashIdx+1, hashIdx) = '' which has no whitespace
    // Actually cursorPos=4 means before='hey #', query='' which is valid
    expect(detectChannelTag('hey #gen', 4)).toBeNull();
  });

  it('returns null for whitespace mid-query', () => {
    expect(detectChannelTag('#some channel', 13)).toBeNull();
  });
});

describe('insertChannelTag', () => {
  it('replaces the match region with #name + space', () => {
    const channel = makeChannel({ name: 'general' });
    const match = { query: 'gen', start: 4, end: 8 };
    const result = insertChannelTag('hey #gen', match, channel);
    expect(result.text).toBe('hey #general ');
    expect(result.cursor).toBe(13); // 4 + '#general '.length
  });

  it('handles match at start of text', () => {
    const channel = makeChannel({ name: 'dev' });
    const match = { query: 'd', start: 0, end: 2 };
    const result = insertChannelTag('#d more text', match, channel);
    expect(result.text).toBe('#dev  more text');
    expect(result.cursor).toBe(5); // '#dev '.length
  });

  it('preserves text after the match', () => {
    const channel = makeChannel({ name: 'random' });
    const match = { query: 'ran', start: 5, end: 9 };
    const result = insertChannelTag('hello#ran world', match, channel);
    expect(result.text).toBe('hello#random  world');
    expect(result.cursor).toBe(13);
  });
});

describe('resolveChannelTags', () => {
  const channels: Channel[] = [
    makeChannel({ id: '100', name: 'general' }),
    makeChannel({ id: '200', name: 'random' }),
    makeChannel({ id: '300', name: 'Dev-Talk' }),
  ];

  it('returns text unchanged when no # present', () => {
    expect(resolveChannelTags('hello world', channels)).toBe('hello world');
  });

  it('replaces known #channel with <#id>', () => {
    expect(resolveChannelTags('go to #general please', channels)).toBe(
      'go to <#100> please',
    );
  });

  it('is case-insensitive', () => {
    expect(resolveChannelTags('#GENERAL and #Random', channels)).toBe(
      '<#100> and <#200>',
    );
  });

  it('leaves unknown channel names unchanged', () => {
    expect(resolveChannelTags('#unknown', channels)).toBe('#unknown');
  });

  it('does not double-encode already-encoded tags', () => {
    // The regex checks for <# prefix — but the pattern is /#(\S+)/
    // which matches <#123> as #123> with the < being before the match.
    // Actually the full match starts at #, so full = '#123>' and it checks
    // full.startsWith('<#') which is false. Let's check actual behavior:
    // Input: '<#100>' — the regex finds '#100>' as full='#100>', name='100>'
    // full.startsWith('<#') is false since full='#100>'
    // So the code looks up '100>' in the map — not found, returns '#100>'
    // The < before remains, so output is '<#100>' — preserved correctly.
    expect(resolveChannelTags('<#100>', channels)).toBe('<#100>');
  });

  it('replaces multiple tags in one string', () => {
    expect(resolveChannelTags('#general and #random', channels)).toBe(
      '<#100> and <#200>',
    );
  });

  it('handles mixed known and unknown tags', () => {
    expect(resolveChannelTags('#general #nope #random', channels)).toBe(
      '<#100> #nope <#200>',
    );
  });

  it('handles case-insensitive match with mixed case name', () => {
    expect(resolveChannelTags('#dev-talk', channels)).toBe('<#300>');
  });
});
