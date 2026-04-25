import React, { createContext, useContext, useEffect, useState, useCallback, ReactNode } from 'react';
import * as readStateApi from '../api/readState';
import type { ChannelKind, NotificationSetting } from '../api/types';

interface ContextValue {
  lastReadByChannel: Map<string, string>;
  notificationSettingByChannel: Map<string, NotificationSetting>;
  // Synchronous getters (return defaults when missing).
  getLastRead: (kind: ChannelKind, channelId: string) => string | null;
  getNotificationSetting: (kind: ChannelKind, channelId: string) => NotificationSetting;
  // Imperative updates (used after a local optimistic action).
  setLastReadLocal: (kind: ChannelKind, channelId: string, messageId: string) => void;
  setNotificationSettingLocal: (kind: ChannelKind, channelId: string, setting: NotificationSetting) => void;
}

const channelKey = (kind: ChannelKind, channelId: string): string => `${kind}:${channelId}`;

const SETTING_FROM_INT: Record<0 | 1 | 2, NotificationSetting> = {
  0: 'ALL',
  1: 'MENTIONS_ONLY',
  2: 'MUTED',
};

const ChannelStateContext = createContext<ContextValue | null>(null);

export function ChannelStateProvider({ children }: { children: ReactNode }): React.ReactElement {
  const [lastReadByChannel, setLastReadByChannel] = useState<Map<string, string>>(new Map());
  const [notificationSettingByChannel, setNotificationSettingByChannel] = useState<Map<string, NotificationSetting>>(new Map());

  // Initial bulk hydration on mount.
  useEffect(() => {
    let cancelled = false;
    readStateApi.fetchAllChannelState().then((rows) => {
      if (cancelled) return;
      const lastReadMap = new Map<string, string>();
      const settingMap = new Map<string, NotificationSetting>();
      for (const row of rows) {
        const key = channelKey(row.channel_kind, row.channel_id);
        if (row.last_read_message_id != null) {
          lastReadMap.set(key, row.last_read_message_id);
        }
        if (row.notification_setting !== 0) {
          // Only store non-default; default (ALL) is implied by absence.
          settingMap.set(key, SETTING_FROM_INT[row.notification_setting]);
        }
      }
      setLastReadByChannel(lastReadMap);
      setNotificationSettingByChannel(settingMap);
    }).catch((err) => {
      console.error('ChannelStateContext: failed to fetch initial state', err);
    });
    return () => { cancelled = true; };
  }, []);

  // Subscribe to WS update CustomEvents to keep maps live.
  useEffect(() => {
    const onReadState = (e: Event) => {
      const detail = (e as CustomEvent<{channel_kind: ChannelKind; channel_id: string; last_read_message_id: string}>).detail;
      setLastReadByChannel((prev) => {
        const next = new Map(prev);
        next.set(channelKey(detail.channel_kind, detail.channel_id), detail.last_read_message_id);
        return next;
      });
    };
    const onNotification = (e: Event) => {
      const detail = (e as CustomEvent<{channel_kind: ChannelKind; channel_id: string; notification_setting: 0 | 1 | 2}>).detail;
      setNotificationSettingByChannel((prev) => {
        const next = new Map(prev);
        next.set(channelKey(detail.channel_kind, detail.channel_id), SETTING_FROM_INT[detail.notification_setting]);
        return next;
      });
    };
    window.addEventListener('parley:channel_read_state', onReadState);
    window.addEventListener('parley:channel_notification', onNotification);
    return () => {
      window.removeEventListener('parley:channel_read_state', onReadState);
      window.removeEventListener('parley:channel_notification', onNotification);
    };
  }, []);

  const getLastRead = useCallback((kind: ChannelKind, channelId: string): string | null => {
    return lastReadByChannel.get(channelKey(kind, channelId)) ?? null;
  }, [lastReadByChannel]);

  const getNotificationSetting = useCallback((kind: ChannelKind, channelId: string): NotificationSetting => {
    return notificationSettingByChannel.get(channelKey(kind, channelId)) ?? 'ALL';
  }, [notificationSettingByChannel]);

  const setLastReadLocal = useCallback((kind: ChannelKind, channelId: string, messageId: string) => {
    setLastReadByChannel((prev) => {
      const next = new Map(prev);
      next.set(channelKey(kind, channelId), messageId);
      return next;
    });
  }, []);

  const setNotificationSettingLocal = useCallback((kind: ChannelKind, channelId: string, setting: NotificationSetting) => {
    setNotificationSettingByChannel((prev) => {
      const next = new Map(prev);
      next.set(channelKey(kind, channelId), setting);
      return next;
    });
  }, []);

  return (
    <ChannelStateContext.Provider value={{
      lastReadByChannel,
      notificationSettingByChannel,
      getLastRead,
      getNotificationSetting,
      setLastReadLocal,
      setNotificationSettingLocal,
    }}>
      {children}
    </ChannelStateContext.Provider>
  );
}

export function useChannelState(): ContextValue {
  const ctx = useContext(ChannelStateContext);
  if (!ctx) {
    throw new Error('useChannelState must be used within ChannelStateProvider');
  }
  return ctx;
}
