// Permission constants — all bigint to handle bits 32+ safely.
// JavaScript's bitwise operators only work on 32-bit integers,
// so we use BigInt throughout for correctness.

// ── Server-only permissions (bits 0–13) ──────────────────────────────────────
export const PERM_ADMINISTRATOR      = 1n << 0n;
export const PERM_MANAGE_SERVER      = 1n << 1n;
export const PERM_MANAGE_ROLES       = 1n << 2n;
export const PERM_MANAGE_CHANNELS    = 1n << 3n;
export const PERM_KICK_MEMBERS       = 1n << 4n;
export const PERM_BAN_MEMBERS        = 1n << 5n;
export const PERM_MANAGE_NICKNAMES   = 1n << 6n;
export const PERM_CHANGE_NICKNAME    = 1n << 7n;
export const PERM_CREATE_INVITE      = 1n << 8n;
export const PERM_VIEW_AUDIT_LOG     = 1n << 9n;
export const PERM_MANAGE_WEBHOOKS    = 1n << 10n;
export const PERM_MANAGE_EXPRESSIONS = 1n << 11n;
export const PERM_MANAGE_EVENTS      = 1n << 12n;
export const PERM_MODERATE_MEMBER    = 1n << 13n;

// ── Channel permissions — Text & Bin (bits 16–31) ────────────────────────────
export const PERM_VIEW_CHANNEL              = 1n << 16n;
export const PERM_SEND_MESSAGES             = 1n << 17n;
export const PERM_EMBED_LINKS               = 1n << 18n;
export const PERM_ATTACH_FILES              = 1n << 19n;
export const PERM_ADD_REACTIONS             = 1n << 20n;
export const PERM_MENTION_EVERYONE          = 1n << 21n;
export const PERM_MANAGE_MESSAGES           = 1n << 22n;
export const PERM_READ_MESSAGE_HISTORY      = 1n << 23n;
export const PERM_USE_EXTERNAL_EMOJI        = 1n << 24n;
export const PERM_PIN_MESSAGES              = 1n << 25n;
export const PERM_MANAGE_THREADS            = 1n << 26n;
export const PERM_CREATE_PUBLIC_THREADS     = 1n << 27n;
export const PERM_SEND_MESSAGES_IN_THREADS  = 1n << 28n;
export const PERM_CREATE_POSTS              = 1n << 29n;
export const PERM_MANAGE_POSTS              = 1n << 30n;
export const PERM_MANAGE_TAGS               = 1n << 31n;

// ── Channel permissions — Voice (bits 32–41) ─────────────────────────────────
export const PERM_CONNECT              = 1n << 32n;
export const PERM_SPEAK                = 1n << 33n;
export const PERM_MUTE_MEMBERS         = 1n << 34n;
export const PERM_DEAFEN_MEMBERS       = 1n << 35n;
export const PERM_MOVE_MEMBERS         = 1n << 36n;
export const PERM_USE_VAD              = 1n << 37n;
export const PERM_PRIORITY_SPEAKER     = 1n << 38n;
export const PERM_STREAM               = 1n << 39n;
export const PERM_USE_SOUNDBOARD       = 1n << 40n;
export const PERM_SEND_VOICE_MESSAGES  = 1n << 41n;

// ── Masks ─────────────────────────────────────────────────────────────────────
export const PERM_ALL          = (1n << 42n) - 1n;
export const PERM_CHANNEL_MASK = PERM_ALL & ~((1n << 16n) - 1n);
export const PERM_SERVER_ONLY_MASK = (1n << 14n) - 1n;

// ── PermOverwrite ─────────────────────────────────────────────────────────────

/** Mirrors the backend db.Overwrite model. target_type: 0 = role, 1 = member. */
export interface PermOverwrite {
  id: string;
  channel_id: string;
  target_type: number;
  target_id: string;
  allow: bigint;
  deny: bigint;
}

// ── Permission categories for UI display ─────────────────────────────────────

export interface PermissionEntry {
  name: string;
  bit: bigint;
  description: string;
}

export interface PermissionCategory {
  label: string;
  permissions: PermissionEntry[];
}

export const PERMISSION_CATEGORIES: PermissionCategory[] = [
  {
    label: 'General',
    permissions: [
      { name: 'Administrator',      bit: PERM_ADMINISTRATOR,    description: 'Grants all permissions and bypasses all channel-specific restrictions.' },
      { name: 'Manage Server',      bit: PERM_MANAGE_SERVER,    description: 'Can change server name, icon, and settings.' },
      { name: 'Manage Roles',       bit: PERM_MANAGE_ROLES,     description: 'Can create, edit, and delete roles below their own.' },
      { name: 'Manage Channels',    bit: PERM_MANAGE_CHANNELS,  description: 'Can create, edit, and delete channels.' },
      { name: 'Kick Members',       bit: PERM_KICK_MEMBERS,     description: 'Can remove members from the server.' },
      { name: 'Ban Members',        bit: PERM_BAN_MEMBERS,      description: 'Can permanently ban members from the server.' },
      { name: 'Manage Nicknames',   bit: PERM_MANAGE_NICKNAMES, description: 'Can change the nicknames of other members.' },
      { name: 'Change Nickname',    bit: PERM_CHANGE_NICKNAME,  description: 'Can change their own nickname.' },
      { name: 'Create Invite',      bit: PERM_CREATE_INVITE,    description: 'Can create invite links for this server.' },
    ],
  },
  {
    label: 'Text',
    permissions: [
      { name: 'View Channel',          bit: PERM_VIEW_CHANNEL,         description: 'Can see and read this channel.' },
      { name: 'Send Messages',         bit: PERM_SEND_MESSAGES,        description: 'Can send messages in text channels.' },
      { name: 'Embed Links',           bit: PERM_EMBED_LINKS,          description: 'Links sent will show embedded previews.' },
      { name: 'Attach Files',          bit: PERM_ATTACH_FILES,         description: 'Can attach files and images to messages.' },
      { name: 'Add Reactions',         bit: PERM_ADD_REACTIONS,        description: 'Can add emoji reactions to messages.' },
      { name: 'Mention Everyone',      bit: PERM_MENTION_EVERYONE,     description: 'Can use @everyone and @here mentions.' },
      { name: 'Manage Messages',       bit: PERM_MANAGE_MESSAGES,      description: 'Can delete or pin messages from others.' },
      { name: 'Read Message History',  bit: PERM_READ_MESSAGE_HISTORY, description: 'Can read messages sent before they joined.' },
      { name: 'Pin Messages',          bit: PERM_PIN_MESSAGES,         description: 'Can pin messages in channels.' },
    ],
  },
  {
    label: 'Bin',
    permissions: [
      { name: 'Create Posts',  bit: PERM_CREATE_POSTS, description: 'Can create posts in Bin channels.' },
      { name: 'Manage Posts',  bit: PERM_MANAGE_POSTS, description: 'Can edit or delete posts from others.' },
      { name: 'Manage Tags',   bit: PERM_MANAGE_TAGS,  description: 'Can create, edit, and delete Bin channel tags.' },
    ],
  },
  {
    label: 'Voice',
    permissions: [
      { name: 'Connect',         bit: PERM_CONNECT,        description: 'Can connect to voice channels.' },
      { name: 'Speak',           bit: PERM_SPEAK,          description: 'Can speak in voice channels.' },
      { name: 'Mute Members',    bit: PERM_MUTE_MEMBERS,   description: 'Can mute members in voice channels.' },
      { name: 'Deafen Members',  bit: PERM_DEAFEN_MEMBERS, description: 'Can deafen members in voice channels.' },
      { name: 'Move Members',    bit: PERM_MOVE_MEMBERS,   description: 'Can move members between voice channels.' },
      { name: 'Use VAD',         bit: PERM_USE_VAD,        description: 'Can use voice activity detection instead of push-to-talk.' },
    ],
  },
];

// ── Computation functions ─────────────────────────────────────────────────────

/** Returns true if all bits in `perm` are set in `perms`. */
export function hasPerm(perms: bigint, perm: bigint): boolean {
  return (perms & perm) === perm;
}

/**
 * Computes server-wide base permissions for a member.
 * Mirrors backend ComputeBasePermissions exactly.
 */
export function computeBasePermissions(
  everyonePerms: bigint,
  memberRolePerms: bigint[],
  isOwner: boolean,
): bigint {
  if (isOwner) return PERM_ALL;
  let perms = everyonePerms;
  for (const rp of memberRolePerms) {
    perms |= rp;
  }
  if (hasPerm(perms, PERM_ADMINISTRATOR)) return PERM_ALL;
  return perms;
}

/**
 * Applies channel permission overwrites to base permissions.
 * Mirrors backend ComputeChannelPermissions exactly.
 *
 * target_type: 0 = role, 1 = member.
 * The IDs in PermOverwrite are strings (snowflakes from the API).
 */
export function computeChannelPermissions(
  basePerms: bigint,
  memberID: string,
  memberRoleIDs: string[],
  everyoneRoleID: string,
  overwrites: PermOverwrite[],
): bigint {
  if (hasPerm(basePerms, PERM_ADMINISTRATOR)) return PERM_ALL;

  let perms = basePerms;

  // Step 1: @everyone role overwrite
  for (const ow of overwrites) {
    if (ow.target_type === 0 && ow.target_id === everyoneRoleID) {
      perms &= ~ow.deny;
      perms |= ow.allow;
      break;
    }
  }

  // Step 2: Role overwrites (combined — allow OR'd, deny OR'd, then applied)
  const roleSet = new Set(memberRoleIDs);
  let roleAllow = 0n;
  let roleDeny  = 0n;
  for (const ow of overwrites) {
    if (ow.target_type === 0 && ow.target_id !== everyoneRoleID && roleSet.has(ow.target_id)) {
      roleAllow |= ow.allow;
      roleDeny  |= ow.deny;
    }
  }
  perms &= ~roleDeny;
  perms |= roleAllow;

  // Step 3: Member-specific overwrite
  for (const ow of overwrites) {
    if (ow.target_type === 1 && ow.target_id === memberID) {
      perms &= ~ow.deny;
      perms |= ow.allow;
      break;
    }
  }

  // Implicit denials (matching backend logic)
  if (!hasPerm(perms, PERM_VIEW_CHANNEL)) {
    perms &= ~PERM_CHANNEL_MASK;
  }
  if (!hasPerm(perms, PERM_SEND_MESSAGES)) {
    perms &= ~(PERM_MENTION_EVERYONE | PERM_ATTACH_FILES | PERM_EMBED_LINKS);
  }
  if (!hasPerm(perms, PERM_CONNECT)) {
    perms &= ~(
      PERM_SPEAK | PERM_MUTE_MEMBERS | PERM_DEAFEN_MEMBERS | PERM_MOVE_MEMBERS |
      PERM_USE_VAD | PERM_PRIORITY_SPEAKER | PERM_STREAM | PERM_USE_SOUNDBOARD |
      PERM_SEND_VOICE_MESSAGES
    );
  }

  return perms;
}

// ── API boundary helpers ──────────────────────────────────────────────────────

/** Convert a number from the API to a bigint permission value. */
export function permFromNumber(n: number): bigint {
  return BigInt(n);
}

/** Convert a bigint permission value back to a number for the API. */
export function permToNumber(b: bigint): number {
  return Number(b);
}
