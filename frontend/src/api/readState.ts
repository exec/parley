// Cross-cutting per-(user, channel) read-state and notification API calls.
// Used by the useChannelReadState and useChannelNotificationSetting hooks
// (frontend/src/hooks/) for both server channels and DM channels.

import { apiClient } from './client';
import type { ChannelKind, NotificationSetting } from './types';

const pathRoot = (kind: ChannelKind) => (kind === 1 ? 'channels' : 'dms');

export async function markRead(kind: ChannelKind, channelId: string, messageId: string): Promise<void> {
  await apiClient.post<void>(`/${pathRoot(kind)}/${channelId}/read`, { message_id: messageId });
}

export async function setNotificationSetting(
  kind: ChannelKind,
  channelId: string,
  setting: NotificationSetting,
): Promise<void> {
  await apiClient.patch<void>(`/${pathRoot(kind)}/${channelId}/notifications`, { setting });
}
