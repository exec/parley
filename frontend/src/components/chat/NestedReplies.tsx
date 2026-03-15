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
 * Finds the display name or username of the author of the parent message.
 * Returns null if the parent is not found in the messages list.
 */
export function getParentAuthor(parentId: string, messages: Message[]): string | null {
  const parent = messages.find((m) => m.id === parentId);
  if (!parent) return null;
  return parent.author_display_name || parent.author_username;
}
