import { useCallback, useEffect, useState } from 'react';
import * as readStateApi from '../api/readState';
import type { ChannelKind, NotificationSetting } from '../api/types';

// Per (kind, channelId) notification setting. Listens to parley:channel_notification
// CustomEvent (dispatched by App.tsx from the WS update event).
export function useChannelNotificationSetting(kind: ChannelKind, channelId: string | null): {
  setting: NotificationSetting;
  setSetting: (s: NotificationSetting) => void;
} {
  const [setting, setSettingState] = useState<NotificationSetting>('ALL');

  useEffect(() => {
    if (!channelId) return;
    const handler = (e: Event) => {
      const detail = (e as CustomEvent<{channel_kind: ChannelKind; channel_id: string; notification_setting: 0 | 1 | 2}>).detail;
      if (detail.channel_kind === kind && detail.channel_id === channelId) {
        const map: Record<0 | 1 | 2, NotificationSetting> = { 0: 'ALL', 1: 'MENTIONS_ONLY', 2: 'MUTED' };
        setSettingState(map[detail.notification_setting]);
      }
    };
    window.addEventListener('parley:channel_notification', handler);
    return () => window.removeEventListener('parley:channel_notification', handler);
  }, [kind, channelId]);

  const setSetting = useCallback((s: NotificationSetting) => {
    if (!channelId) return;
    setSettingState(s); // optimistic
    readStateApi.setNotificationSetting(kind, channelId, s).catch((err) => {
      console.error('setNotificationSetting failed', err);
    });
  }, [kind, channelId]);

  return { setting, setSetting };
}
