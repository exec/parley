import { describe, it, expect } from 'vitest';
import {
  // Permission constants
  PERM_ADMINISTRATOR,
  PERM_MANAGE_SERVER,
  PERM_MANAGE_ROLES,
  PERM_MANAGE_CHANNELS,
  PERM_KICK_MEMBERS,
  PERM_BAN_MEMBERS,
  PERM_MANAGE_NICKNAMES,
  PERM_CHANGE_NICKNAME,
  PERM_CREATE_INVITE,
  PERM_VIEW_AUDIT_LOG,
  PERM_MANAGE_WEBHOOKS,
  PERM_MANAGE_EXPRESSIONS,
  PERM_MANAGE_EVENTS,
  PERM_MODERATE_MEMBER,
  PERM_VIEW_CHANNEL,
  PERM_SEND_MESSAGES,
  PERM_EMBED_LINKS,
  PERM_ATTACH_FILES,
  PERM_ADD_REACTIONS,
  PERM_MENTION_EVERYONE,
  PERM_MANAGE_MESSAGES,
  PERM_READ_MESSAGE_HISTORY,
  PERM_USE_EXTERNAL_EMOJI,
  PERM_PIN_MESSAGES,
  PERM_MANAGE_THREADS,
  PERM_CREATE_PUBLIC_THREADS,
  PERM_SEND_MESSAGES_IN_THREADS,
  PERM_CREATE_POSTS,
  PERM_MANAGE_POSTS,
  PERM_MANAGE_TAGS,
  PERM_CONNECT,
  PERM_SPEAK,
  PERM_MUTE_MEMBERS,
  PERM_DEAFEN_MEMBERS,
  PERM_MOVE_MEMBERS,
  PERM_USE_VAD,
  PERM_PRIORITY_SPEAKER,
  PERM_STREAM,
  PERM_USE_SOUNDBOARD,
  PERM_SEND_VOICE_MESSAGES,
  // Masks
  PERM_ALL,
  PERM_CHANNEL_MASK,
  PERM_SERVER_ONLY_MASK,
  // Types
  type PermOverwrite,
  // Functions
  hasPerm,
  computeBasePermissions,
  computeChannelPermissions,
  permFromNumber,
  permToNumber,
  // UI categories
  PERMISSION_CATEGORIES,
} from '../permissions';

// ---------------------------------------------------------------------------
// Permission constants
// ---------------------------------------------------------------------------

describe('permission constants', () => {
  it('each permission is a unique power of two', () => {
    const allPerms = [
      PERM_ADMINISTRATOR, PERM_MANAGE_SERVER, PERM_MANAGE_ROLES,
      PERM_MANAGE_CHANNELS, PERM_KICK_MEMBERS, PERM_BAN_MEMBERS,
      PERM_MANAGE_NICKNAMES, PERM_CHANGE_NICKNAME, PERM_CREATE_INVITE,
      PERM_VIEW_AUDIT_LOG, PERM_MANAGE_WEBHOOKS, PERM_MANAGE_EXPRESSIONS,
      PERM_MANAGE_EVENTS, PERM_MODERATE_MEMBER,
      PERM_VIEW_CHANNEL, PERM_SEND_MESSAGES, PERM_EMBED_LINKS,
      PERM_ATTACH_FILES, PERM_ADD_REACTIONS, PERM_MENTION_EVERYONE,
      PERM_MANAGE_MESSAGES, PERM_READ_MESSAGE_HISTORY, PERM_USE_EXTERNAL_EMOJI,
      PERM_PIN_MESSAGES, PERM_MANAGE_THREADS, PERM_CREATE_PUBLIC_THREADS,
      PERM_SEND_MESSAGES_IN_THREADS, PERM_CREATE_POSTS, PERM_MANAGE_POSTS,
      PERM_MANAGE_TAGS,
      PERM_CONNECT, PERM_SPEAK, PERM_MUTE_MEMBERS, PERM_DEAFEN_MEMBERS,
      PERM_MOVE_MEMBERS, PERM_USE_VAD, PERM_PRIORITY_SPEAKER, PERM_STREAM,
      PERM_USE_SOUNDBOARD, PERM_SEND_VOICE_MESSAGES,
    ];

    // Each must be a power of two (exactly one bit set)
    for (const p of allPerms) {
      expect(p > 0n).toBe(true);
      expect(p & (p - 1n)).toBe(0n);
    }

    // All unique
    const set = new Set(allPerms);
    expect(set.size).toBe(allPerms.length);
  });

  it('server-only bits are in range 0-13', () => {
    const serverPerms = [
      PERM_ADMINISTRATOR, PERM_MANAGE_SERVER, PERM_MANAGE_ROLES,
      PERM_MANAGE_CHANNELS, PERM_KICK_MEMBERS, PERM_BAN_MEMBERS,
      PERM_MANAGE_NICKNAMES, PERM_CHANGE_NICKNAME, PERM_CREATE_INVITE,
      PERM_VIEW_AUDIT_LOG, PERM_MANAGE_WEBHOOKS, PERM_MANAGE_EXPRESSIONS,
      PERM_MANAGE_EVENTS, PERM_MODERATE_MEMBER,
    ];
    for (const p of serverPerms) {
      expect(p < (1n << 14n)).toBe(true);
    }
  });

  it('channel text bits are in range 16-31', () => {
    const textPerms = [
      PERM_VIEW_CHANNEL, PERM_SEND_MESSAGES, PERM_EMBED_LINKS,
      PERM_ATTACH_FILES, PERM_ADD_REACTIONS, PERM_MENTION_EVERYONE,
      PERM_MANAGE_MESSAGES, PERM_READ_MESSAGE_HISTORY, PERM_USE_EXTERNAL_EMOJI,
      PERM_PIN_MESSAGES, PERM_MANAGE_THREADS, PERM_CREATE_PUBLIC_THREADS,
      PERM_SEND_MESSAGES_IN_THREADS, PERM_CREATE_POSTS, PERM_MANAGE_POSTS,
      PERM_MANAGE_TAGS,
    ];
    for (const p of textPerms) {
      expect(p >= (1n << 16n)).toBe(true);
      expect(p <= (1n << 31n)).toBe(true);
    }
  });

  it('voice bits are in range 32-41', () => {
    const voicePerms = [
      PERM_CONNECT, PERM_SPEAK, PERM_MUTE_MEMBERS, PERM_DEAFEN_MEMBERS,
      PERM_MOVE_MEMBERS, PERM_USE_VAD, PERM_PRIORITY_SPEAKER, PERM_STREAM,
      PERM_USE_SOUNDBOARD, PERM_SEND_VOICE_MESSAGES,
    ];
    for (const p of voicePerms) {
      expect(p >= (1n << 32n)).toBe(true);
      expect(p <= (1n << 41n)).toBe(true);
    }
  });
});

// ---------------------------------------------------------------------------
// Masks
// ---------------------------------------------------------------------------

describe('permission masks', () => {
  it('PERM_ALL covers all 42 bits', () => {
    expect(PERM_ALL).toBe((1n << 42n) - 1n);
  });

  it('PERM_CHANNEL_MASK excludes server-only bits (0-15)', () => {
    // bits 0-15 should be zero in PERM_CHANNEL_MASK
    expect(PERM_CHANNEL_MASK & ((1n << 16n) - 1n)).toBe(0n);
    // bits 16+ should all be set
    expect(PERM_CHANNEL_MASK).toBe(PERM_ALL & ~((1n << 16n) - 1n));
  });

  it('PERM_SERVER_ONLY_MASK covers bits 0-13', () => {
    expect(PERM_SERVER_ONLY_MASK).toBe((1n << 14n) - 1n);
  });

  it('PERM_CHANNEL_MASK and PERM_SERVER_ONLY_MASK do not overlap', () => {
    expect(PERM_CHANNEL_MASK & PERM_SERVER_ONLY_MASK).toBe(0n);
  });
});

// ---------------------------------------------------------------------------
// hasPerm
// ---------------------------------------------------------------------------

describe('hasPerm', () => {
  it('returns true when single permission is set', () => {
    expect(hasPerm(PERM_ADMINISTRATOR, PERM_ADMINISTRATOR)).toBe(true);
  });

  it('returns false when single permission is not set', () => {
    expect(hasPerm(PERM_MANAGE_SERVER, PERM_ADMINISTRATOR)).toBe(false);
  });

  it('returns true when checking a subset of set permissions', () => {
    const perms = PERM_VIEW_CHANNEL | PERM_SEND_MESSAGES | PERM_CONNECT;
    expect(hasPerm(perms, PERM_VIEW_CHANNEL | PERM_SEND_MESSAGES)).toBe(true);
  });

  it('returns false when only some of the checked bits are set', () => {
    const perms = PERM_VIEW_CHANNEL;
    expect(hasPerm(perms, PERM_VIEW_CHANNEL | PERM_SEND_MESSAGES)).toBe(false);
  });

  it('returns true for zero perm (vacuous truth)', () => {
    expect(hasPerm(0n, 0n)).toBe(true);
  });

  it('works with permissions above bit 31 (bigint required)', () => {
    const perms = PERM_CONNECT | PERM_SPEAK | PERM_SEND_VOICE_MESSAGES;
    expect(hasPerm(perms, PERM_SEND_VOICE_MESSAGES)).toBe(true);
    expect(hasPerm(perms, PERM_USE_SOUNDBOARD)).toBe(false);
  });

  it('PERM_ALL has every permission', () => {
    expect(hasPerm(PERM_ALL, PERM_ADMINISTRATOR)).toBe(true);
    expect(hasPerm(PERM_ALL, PERM_SEND_VOICE_MESSAGES)).toBe(true);
    expect(hasPerm(PERM_ALL, PERM_ALL)).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// computeBasePermissions
// ---------------------------------------------------------------------------

describe('computeBasePermissions', () => {
  it('returns PERM_ALL for owner regardless of role perms', () => {
    expect(computeBasePermissions(0n, [], true)).toBe(PERM_ALL);
    expect(computeBasePermissions(0n, [PERM_VIEW_CHANNEL], true)).toBe(PERM_ALL);
  });

  it('returns everyone perms when member has no additional roles', () => {
    const everyone = PERM_VIEW_CHANNEL | PERM_SEND_MESSAGES;
    expect(computeBasePermissions(everyone, [], false)).toBe(everyone);
  });

  it('ORs role permissions with everyone perms', () => {
    const everyone = PERM_VIEW_CHANNEL;
    const role1 = PERM_SEND_MESSAGES;
    const role2 = PERM_ATTACH_FILES;
    const result = computeBasePermissions(everyone, [role1, role2], false);
    expect(result).toBe(PERM_VIEW_CHANNEL | PERM_SEND_MESSAGES | PERM_ATTACH_FILES);
  });

  it('returns PERM_ALL if any role grants ADMINISTRATOR', () => {
    const everyone = PERM_VIEW_CHANNEL;
    const adminRole = PERM_ADMINISTRATOR | PERM_MANAGE_SERVER;
    const result = computeBasePermissions(everyone, [adminRole], false);
    expect(result).toBe(PERM_ALL);
  });

  it('returns PERM_ALL if everyone role has ADMINISTRATOR', () => {
    const result = computeBasePermissions(PERM_ADMINISTRATOR, [], false);
    expect(result).toBe(PERM_ALL);
  });

  it('handles multiple roles with overlapping bits', () => {
    const everyone = PERM_VIEW_CHANNEL | PERM_SEND_MESSAGES;
    const role1 = PERM_SEND_MESSAGES | PERM_EMBED_LINKS;
    const role2 = PERM_EMBED_LINKS | PERM_ATTACH_FILES;
    const result = computeBasePermissions(everyone, [role1, role2], false);
    expect(result).toBe(
      PERM_VIEW_CHANNEL | PERM_SEND_MESSAGES | PERM_EMBED_LINKS | PERM_ATTACH_FILES,
    );
  });

  it('returns 0 when everyone and all roles are 0', () => {
    expect(computeBasePermissions(0n, [0n, 0n], false)).toBe(0n);
  });
});

// ---------------------------------------------------------------------------
// computeChannelPermissions
// ---------------------------------------------------------------------------

describe('computeChannelPermissions', () => {
  const memberId = 'member-1';
  const everyoneRoleId = 'everyone-role';

  function ow(
    targetType: number,
    targetId: string,
    allow: bigint,
    deny: bigint,
  ): PermOverwrite {
    return {
      id: `ow-${targetId}`,
      channel_id: 'ch-1',
      target_type: targetType,
      target_id: targetId,
      allow,
      deny,
    };
  }

  it('returns PERM_ALL for admins regardless of overwrites', () => {
    const overwrites = [ow(0, everyoneRoleId, 0n, PERM_VIEW_CHANNEL)];
    const result = computeChannelPermissions(
      PERM_ALL, memberId, [], everyoneRoleId, overwrites,
    );
    expect(result).toBe(PERM_ALL);
  });

  describe('everyone role overwrite (step 1)', () => {
    it('applies deny from everyone overwrite', () => {
      const base = PERM_VIEW_CHANNEL | PERM_SEND_MESSAGES;
      const overwrites = [ow(0, everyoneRoleId, 0n, PERM_SEND_MESSAGES)];
      const result = computeChannelPermissions(
        base, memberId, [], everyoneRoleId, overwrites,
      );
      expect(hasPerm(result, PERM_SEND_MESSAGES)).toBe(false);
      expect(hasPerm(result, PERM_VIEW_CHANNEL)).toBe(true);
    });

    it('applies allow from everyone overwrite', () => {
      const base = PERM_VIEW_CHANNEL;
      const overwrites = [ow(0, everyoneRoleId, PERM_SEND_MESSAGES, 0n)];
      const result = computeChannelPermissions(
        base, memberId, [], everyoneRoleId, overwrites,
      );
      expect(hasPerm(result, PERM_SEND_MESSAGES)).toBe(true);
    });

    it('deny is applied before allow in everyone overwrite', () => {
      // If the same bit is in both allow and deny, allow wins (deny first, then allow OR)
      // Need VIEW_CHANNEL in base to avoid implicit denial stripping all channel bits
      const base = PERM_VIEW_CHANNEL;
      const overwrites = [
        ow(0, everyoneRoleId, PERM_SEND_MESSAGES, PERM_SEND_MESSAGES),
      ];
      const result = computeChannelPermissions(
        base, memberId, [], everyoneRoleId, overwrites,
      );
      expect(hasPerm(result, PERM_SEND_MESSAGES)).toBe(true);
    });
  });

  describe('role overwrites (step 2)', () => {
    it('applies combined role allow/deny', () => {
      const base = PERM_VIEW_CHANNEL | PERM_SEND_MESSAGES | PERM_EMBED_LINKS;
      const roleA = 'role-a';
      const roleB = 'role-b';
      const overwrites = [
        ow(0, roleA, PERM_PIN_MESSAGES, PERM_SEND_MESSAGES),
        ow(0, roleB, PERM_ADD_REACTIONS, PERM_EMBED_LINKS),
      ];
      const result = computeChannelPermissions(
        base, memberId, [roleA, roleB], everyoneRoleId, overwrites,
      );
      // SEND_MESSAGES and EMBED_LINKS denied
      expect(hasPerm(result, PERM_SEND_MESSAGES)).toBe(false);
      expect(hasPerm(result, PERM_EMBED_LINKS)).toBe(false);
      // PIN_MESSAGES and ADD_REACTIONS allowed
      expect(hasPerm(result, PERM_PIN_MESSAGES)).toBe(true);
      expect(hasPerm(result, PERM_ADD_REACTIONS)).toBe(true);
      // VIEW_CHANNEL untouched
      expect(hasPerm(result, PERM_VIEW_CHANNEL)).toBe(true);
    });

    it('role allow overrides role deny for the same bit (OR semantics)', () => {
      const base = PERM_VIEW_CHANNEL;
      const roleA = 'role-a';
      const roleB = 'role-b';
      const overwrites = [
        ow(0, roleA, 0n, PERM_SEND_MESSAGES),       // deny SEND
        ow(0, roleB, PERM_SEND_MESSAGES, 0n),        // allow SEND
      ];
      const result = computeChannelPermissions(
        base, memberId, [roleA, roleB], everyoneRoleId, overwrites,
      );
      // Combined: deny applied first, then allow OR'd — allow wins
      expect(hasPerm(result, PERM_SEND_MESSAGES)).toBe(true);
    });

    it('ignores role overwrites for roles the member does not have', () => {
      const base = PERM_VIEW_CHANNEL;
      const roleA = 'role-a';
      const overwrites = [ow(0, roleA, PERM_SEND_MESSAGES, 0n)];
      const result = computeChannelPermissions(
        base, memberId, [], everyoneRoleId, overwrites,
      );
      expect(hasPerm(result, PERM_SEND_MESSAGES)).toBe(false);
    });

    it('does not treat everyone role as a regular role overwrite', () => {
      // The everyone role overwrite is handled in step 1, not step 2
      const base = PERM_VIEW_CHANNEL;
      const overwrites = [ow(0, everyoneRoleId, PERM_SEND_MESSAGES, 0n)];
      // memberRoleIDs includes everyone — but the code explicitly skips it
      const result = computeChannelPermissions(
        base, memberId, [everyoneRoleId], everyoneRoleId, overwrites,
      );
      // Allow from step 1 should still apply
      expect(hasPerm(result, PERM_SEND_MESSAGES)).toBe(true);
    });
  });

  describe('member-specific overwrite (step 3)', () => {
    it('member overwrite overrides role denials', () => {
      const base = PERM_VIEW_CHANNEL | PERM_SEND_MESSAGES;
      const roleA = 'role-a';
      const overwrites = [
        ow(0, roleA, 0n, PERM_SEND_MESSAGES),           // role denies
        ow(1, memberId, PERM_SEND_MESSAGES, 0n),         // member allows
      ];
      const result = computeChannelPermissions(
        base, memberId, [roleA], everyoneRoleId, overwrites,
      );
      expect(hasPerm(result, PERM_SEND_MESSAGES)).toBe(true);
    });

    it('member deny overrides role allows', () => {
      const base = PERM_VIEW_CHANNEL;
      const roleA = 'role-a';
      const overwrites = [
        ow(0, roleA, PERM_SEND_MESSAGES, 0n),           // role allows
        ow(1, memberId, 0n, PERM_SEND_MESSAGES),         // member denies
      ];
      const result = computeChannelPermissions(
        base, memberId, [roleA], everyoneRoleId, overwrites,
      );
      expect(hasPerm(result, PERM_SEND_MESSAGES)).toBe(false);
    });

    it('does not apply member overwrite for a different member', () => {
      const base = PERM_VIEW_CHANNEL;
      const overwrites = [
        ow(1, 'other-member', PERM_SEND_MESSAGES, 0n),
      ];
      const result = computeChannelPermissions(
        base, memberId, [], everyoneRoleId, overwrites,
      );
      expect(hasPerm(result, PERM_SEND_MESSAGES)).toBe(false);
    });
  });

  describe('implicit denials', () => {
    it('denies all channel perms when VIEW_CHANNEL is missing', () => {
      const base = PERM_SEND_MESSAGES | PERM_CONNECT | PERM_SPEAK;
      const result = computeChannelPermissions(
        base, memberId, [], everyoneRoleId, [],
      );
      // Without VIEW_CHANNEL, all channel-mask bits are stripped
      expect(result & PERM_CHANNEL_MASK).toBe(0n);
    });

    it('denies MENTION_EVERYONE, ATTACH_FILES, EMBED_LINKS when SEND_MESSAGES is missing', () => {
      const base = PERM_VIEW_CHANNEL | PERM_MENTION_EVERYONE | PERM_ATTACH_FILES | PERM_EMBED_LINKS;
      const result = computeChannelPermissions(
        base, memberId, [], everyoneRoleId, [],
      );
      expect(hasPerm(result, PERM_VIEW_CHANNEL)).toBe(true);
      expect(hasPerm(result, PERM_MENTION_EVERYONE)).toBe(false);
      expect(hasPerm(result, PERM_ATTACH_FILES)).toBe(false);
      expect(hasPerm(result, PERM_EMBED_LINKS)).toBe(false);
    });

    it('denies voice sub-perms when CONNECT is missing', () => {
      const voiceSubPerms =
        PERM_SPEAK | PERM_MUTE_MEMBERS | PERM_DEAFEN_MEMBERS |
        PERM_MOVE_MEMBERS | PERM_USE_VAD | PERM_PRIORITY_SPEAKER |
        PERM_STREAM | PERM_USE_SOUNDBOARD | PERM_SEND_VOICE_MESSAGES;
      const base = PERM_VIEW_CHANNEL | voiceSubPerms;
      const result = computeChannelPermissions(
        base, memberId, [], everyoneRoleId, [],
      );
      expect(result & voiceSubPerms).toBe(0n);
      expect(hasPerm(result, PERM_VIEW_CHANNEL)).toBe(true);
    });

    it('preserves voice sub-perms when CONNECT is present', () => {
      const base = PERM_VIEW_CHANNEL | PERM_CONNECT | PERM_SPEAK | PERM_STREAM;
      const result = computeChannelPermissions(
        base, memberId, [], everyoneRoleId, [],
      );
      expect(hasPerm(result, PERM_SPEAK)).toBe(true);
      expect(hasPerm(result, PERM_STREAM)).toBe(true);
    });

    it('implicit denial cascade: no VIEW_CHANNEL strips everything including voice', () => {
      const base = PERM_SEND_MESSAGES | PERM_CONNECT | PERM_SPEAK | PERM_ADMINISTRATOR & 0n;
      // No VIEW_CHANNEL, no server perms — just channel bits
      const allChannel = PERM_SEND_MESSAGES | PERM_CONNECT | PERM_SPEAK;
      const result = computeChannelPermissions(
        allChannel, memberId, [], everyoneRoleId, [],
      );
      expect(result & PERM_CHANNEL_MASK).toBe(0n);
    });
  });

  describe('full overwrite chain ordering', () => {
    it('everyone -> role -> member overwrites applied in correct order', () => {
      const base = PERM_VIEW_CHANNEL | PERM_SEND_MESSAGES;
      const roleA = 'role-a';

      const overwrites = [
        // Step 1: everyone denies SEND_MESSAGES
        ow(0, everyoneRoleId, 0n, PERM_SEND_MESSAGES),
        // Step 2: role re-allows SEND_MESSAGES
        ow(0, roleA, PERM_SEND_MESSAGES, 0n),
        // Step 3: member denies SEND_MESSAGES again
        ow(1, memberId, 0n, PERM_SEND_MESSAGES),
      ];

      const result = computeChannelPermissions(
        base, memberId, [roleA], everyoneRoleId, overwrites,
      );
      // Member deny is final
      expect(hasPerm(result, PERM_SEND_MESSAGES)).toBe(false);
      expect(hasPerm(result, PERM_VIEW_CHANNEL)).toBe(true);
    });
  });

  describe('no overwrites', () => {
    it('returns base perms (minus implicit denials) with empty overwrites', () => {
      const base = PERM_VIEW_CHANNEL | PERM_SEND_MESSAGES | PERM_CONNECT | PERM_SPEAK;
      const result = computeChannelPermissions(
        base, memberId, [], everyoneRoleId, [],
      );
      expect(result).toBe(base);
    });
  });
});

// ---------------------------------------------------------------------------
// permFromNumber / permToNumber
// ---------------------------------------------------------------------------

describe('permFromNumber', () => {
  it('converts 0 to 0n', () => {
    expect(permFromNumber(0)).toBe(0n);
  });

  it('converts a positive number to bigint', () => {
    expect(permFromNumber(65536)).toBe(65536n);
  });

  it('roundtrips with permToNumber for safe integers', () => {
    const n = 0x1FFFFF; // 21 bits, safe integer
    expect(permToNumber(permFromNumber(n))).toBe(n);
  });
});

describe('permToNumber', () => {
  it('converts 0n to 0', () => {
    expect(permToNumber(0n)).toBe(0);
  });

  it('converts bigint to number', () => {
    expect(permToNumber(65536n)).toBe(65536);
  });
});

// ---------------------------------------------------------------------------
// PERMISSION_CATEGORIES
// ---------------------------------------------------------------------------

describe('PERMISSION_CATEGORIES', () => {
  it('has 4 categories: General, Text, Bin, Voice', () => {
    const labels = PERMISSION_CATEGORIES.map((c) => c.label);
    expect(labels).toEqual(['General', 'Text', 'Bin', 'Voice']);
  });

  it('every entry has a non-empty name, description, and non-zero bit', () => {
    for (const cat of PERMISSION_CATEGORIES) {
      for (const entry of cat.permissions) {
        expect(entry.name.length).toBeGreaterThan(0);
        expect(entry.description.length).toBeGreaterThan(0);
        expect(entry.bit > 0n).toBe(true);
      }
    }
  });

  it('all bits across categories are unique', () => {
    const allBits: bigint[] = [];
    for (const cat of PERMISSION_CATEGORIES) {
      for (const entry of cat.permissions) {
        allBits.push(entry.bit);
      }
    }
    const set = new Set(allBits);
    expect(set.size).toBe(allBits.length);
  });

  it('General category only contains server-scope permissions', () => {
    const general = PERMISSION_CATEGORIES.find((c) => c.label === 'General')!;
    for (const entry of general.permissions) {
      expect(entry.bit < (1n << 16n)).toBe(true);
    }
  });

  it('Voice category only contains voice-scope permissions (bits 32+)', () => {
    const voice = PERMISSION_CATEGORIES.find((c) => c.label === 'Voice')!;
    for (const entry of voice.permissions) {
      expect(entry.bit >= (1n << 32n)).toBe(true);
    }
  });
});
