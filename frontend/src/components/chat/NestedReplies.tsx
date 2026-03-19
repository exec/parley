import { Message } from '../../api/types';

export interface MessageTreeNode {
  message: Message;
  replies: Message[];
}

/**
 * Builds a one-level-deep reply tree from a flat message list using parent_id.
 * Returns top-level messages (no parent_id) each paired with their direct replies.
 */
export function buildReplyTree(messages: Message[]): MessageTreeNode[] {
  const topLevel: Message[] = [];
  const byParent = new Map<string, Message[]>();

  for (const msg of messages) {
    if (msg.parent_id) {
      const siblings = byParent.get(msg.parent_id) ?? [];
      siblings.push(msg);
      byParent.set(msg.parent_id, siblings);
    } else {
      topLevel.push(msg);
    }
  }

  return topLevel.map((msg) => ({
    message: msg,
    replies: byParent.get(msg.id) ?? [],
  }));
}

/**
 * Returns the display name of the author of a reply's parent message.
 * Prefers parent_author_display_name/username embedded on the message itself
 * (always populated by the API), then falls back to searching the messages list
 * for messages loaded in the current window.
 */
export function getParentAuthor(msg: Message, messages: Message[]): string | null {
  if (msg.parent_author_display_name) return msg.parent_author_display_name;
  if (msg.parent_author_username) return msg.parent_author_username;
  const parent = messages.find((m) => m.id === msg.parent_id);
  if (!parent) return null;
  return parent.author_display_name || parent.author_username;
}

/** Returns a single-line preview of the parent message content, stripped of newlines. */
export function getParentPreview(msg: Message, messages: Message[]): string | null {
  const parent = messages.find((m) => m.id === msg.parent_id);
  if (!parent?.content) return null;
  return parent.content.replace(/\s*\n\s*/g, ' ').trim();
}
