import React, { createContext, useContext, useReducer, useCallback, useEffect, useRef, useState, MutableRefObject } from 'react';

function useLatest<T>(value: T): MutableRefObject<T> {
  const ref = useRef<T>(value);
  useEffect(() => { ref.current = value; }, [value]);
  return ref;
}
import { ringDm, acceptCall, declineCall, cancelCall, type ActiveRing, type RingCaller } from '../api/calls';
import { invoke } from '@tauri-apps/api/core';
import { listen } from '@tauri-apps/api/event';
import { getCurrentWindow } from '@tauri-apps/api/window';
import { platform } from '@tauri-apps/plugin-os';

const isTauri = typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;

export type CallState = 'idle' | 'outgoing' | 'connecting' | 'connected';

export interface IncomingRing {
  ring_id: string;
  vc: string;
  caller: RingCaller;
  started_at_ms: number;
}

export interface OutgoingTarget {
  user_id: string;
  username: string;
  display_name?: string;
  avatar_url?: string;
}

export interface CallContextValue {
  state: CallState;
  activeVc: string | null;
  activeRingId: string | null;
  incomingQueue: IncomingRing[];
  floatingMode: boolean;
  outgoingTarget?: OutgoingTarget;
  isDesktopTauri: boolean;
  mainFocused: boolean;
  initiate: (dmChannelId: string | number, target: OutgoingTarget) => Promise<void>;
  accept: (ring: IncomingRing) => Promise<void>;
  decline: (ring: IncomingRing) => Promise<void>;
  cancel: () => Promise<void>;
  notifyConnected: () => void;
  notifyDisconnected: () => void;
  setFloatingMode: (floating: boolean) => void;
  receiveCallRing: (payload: { vc: string; ring_id: string; caller: RingCaller; started_at_ms: number }) => void;
  receiveCallAccept: (payload: { ring_id: string; accepter_user_id?: string }) => void;
  receiveCallDecline: (payload: { ring_id: string; decliner_user_id?: string }) => void;
  receiveCallCancel: (payload: { ring_id: string }) => void;
  receiveCallTimeout: (payload: { ring_id: string }) => void;
}

interface Store {
  state: CallState;
  activeVc: string | null;
  activeRingId: string | null;
  incomingQueue: IncomingRing[];
  floatingMode: boolean;
  outgoingTarget?: OutgoingTarget;
}

type Action =
  | { type: 'set'; state: CallState; vc?: string | null; ringId?: string | null; outgoingTarget?: OutgoingTarget | null }
  | { type: 'enqueue'; ring: IncomingRing }
  | { type: 'dequeue'; ring_id: string }
  | { type: 'set_floating'; floating: boolean };

function reducer(store: Store, action: Action): Store {
  switch (action.type) {
    case 'set':
      return {
        ...store,
        state: action.state,
        activeVc: action.vc !== undefined ? action.vc : store.activeVc,
        activeRingId: action.ringId !== undefined ? action.ringId : store.activeRingId,
        outgoingTarget: action.outgoingTarget !== undefined
          ? (action.outgoingTarget ?? undefined)
          : store.outgoingTarget,
      };
    case 'enqueue':
      if (store.incomingQueue.some(r => r.ring_id === action.ring.ring_id)) return store;
      return { ...store, incomingQueue: [...store.incomingQueue, action.ring] };
    case 'dequeue':
      return { ...store, incomingQueue: store.incomingQueue.filter(r => r.ring_id !== action.ring_id) };
    case 'set_floating':
      return { ...store, floatingMode: action.floating };
    default:
      return store;
  }
}

const CallContext = createContext<CallContextValue | null>(null);

interface CallProviderProps {
  children: React.ReactNode;
  bootRings?: ActiveRing[];
}

export const CallProvider: React.FC<CallProviderProps> = ({ children, bootRings }) => {
  const [store, dispatch] = useReducer(reducer, {
    state: 'idle',
    activeVc: null,
    activeRingId: null,
    incomingQueue: [],
    floatingMode: false,
    outgoingTarget: undefined,
  });

  const bootApplied = useRef(false);
  useEffect(() => {
    if (bootApplied.current || !bootRings?.length) return;
    bootApplied.current = true;
    for (const ring of bootRings) {
      dispatch({ type: 'enqueue', ring });
    }
  }, [bootRings]);

  // Secondary ring windows are desktop-only. Mobile Tauri builds fall back to in-app modal.
  const [isDesktopTauri, setIsDesktopTauri] = useState<boolean>(false);
  useEffect(() => {
    if (!isTauri) return;
    try {
      const p = platform();
      setIsDesktopTauri(p === 'macos' || p === 'windows' || p === 'linux');
    } catch {
      setIsDesktopTauri(false);
    }
  }, []);

  const [mainFocused, setMainFocused] = useState<boolean>(true);
  useEffect(() => {
    if (!isTauri) return;
    let unlistenFocus: undefined | (() => void);
    getCurrentWindow().isFocused().then(setMainFocused).catch(() => {});
    getCurrentWindow().onFocusChanged(({ payload }) => setMainFocused(payload)).then(fn => { unlistenFocus = fn; }).catch(() => {});
    return () => { unlistenFocus?.(); };
  }, []);

  const initiate = useCallback(async (dmChannelId: string | number, target: OutgoingTarget) => {
    const vc = `dm:${dmChannelId}`;
    dispatch({ type: 'set', state: 'outgoing', vc, ringId: null, outgoingTarget: target });
    try {
      const { ring_id } = await ringDm(dmChannelId);
      dispatch({ type: 'set', state: 'outgoing', ringId: ring_id });
    } catch {
      dispatch({ type: 'set', state: 'idle', vc: null, ringId: null, outgoingTarget: null });
    }
  }, []);

  const accept = useCallback(async (ring: IncomingRing) => {
    dispatch({ type: 'dequeue', ring_id: ring.ring_id });
    dispatch({ type: 'set', state: 'connecting', vc: ring.vc, ringId: ring.ring_id });
    const dmChannelId = ring.vc.replace(/^dm:/, '');
    try {
      await acceptCall(dmChannelId, ring.ring_id);
    } catch {
      dispatch({ type: 'set', state: 'idle', vc: null, ringId: null });
    }
  }, []);

  const decline = useCallback(async (ring: IncomingRing) => {
    dispatch({ type: 'dequeue', ring_id: ring.ring_id });
    const dmChannelId = ring.vc.replace(/^dm:/, '');
    try {
      await declineCall(dmChannelId, ring.ring_id);
    } catch {
      // best-effort
    }
  }, []);

  const cancel = useCallback(async () => {
    const { activeVc, activeRingId } = store;
    if (!activeRingId) return;
    dispatch({ type: 'set', state: 'idle', vc: null, ringId: null, outgoingTarget: null });
    if (activeVc) {
      const dmChannelId = activeVc.replace(/^dm:/, '');
      try {
        await cancelCall(dmChannelId, activeRingId);
      } catch {
        // best-effort
      }
    }
  }, [store]);

  const notifyConnected = useCallback(() => {
    dispatch({ type: 'set', state: 'connected' });
  }, []);

  const notifyDisconnected = useCallback(() => {
    dispatch({ type: 'set', state: 'idle', vc: null, ringId: null, outgoingTarget: null });
  }, []);

  const setFloatingMode = useCallback((floating: boolean) => {
    dispatch({ type: 'set_floating', floating });
  }, []);

  // Spawn / dismiss secondary ring windows based on focus state and queue
  useEffect(() => {
    if (!isDesktopTauri) return;
    if (mainFocused) {
      store.incomingQueue.forEach(r => {
        invoke('dismiss_ring_window', { ringId: r.ring_id }).catch(() => {});
      });
      return;
    }
    store.incomingQueue.forEach(r => {
      invoke('spawn_ring_window', {
        args: {
          ring_id: r.ring_id,
          vc: r.vc,
          caller_username: r.caller.username,
          caller_display_name: r.caller.display_name,
          caller_avatar_url: r.caller.avatar_url || null,
          group_name: null,
        },
      }).catch(() => {});
    });
  }, [mainFocused, store.incomingQueue, isDesktopTauri]);

  // Dismiss ring windows whose rings have been resolved (left the queue)
  const prevQueueIds = useRef<Set<string>>(new Set());
  useEffect(() => {
    if (!isDesktopTauri) return;
    const currentIds = new Set(store.incomingQueue.map(r => r.ring_id));
    prevQueueIds.current.forEach(id => {
      if (!currentIds.has(id)) {
        invoke('dismiss_ring_window', { ringId: id }).catch(() => {});
      }
    });
    prevQueueIds.current = currentIds;
  }, [store.incomingQueue, isDesktopTauri]);

  // Handle accept/decline from the secondary ring window.
  // Refs keep handlers current without re-registering listeners on every queue change.
  const queueRef = useLatest(store.incomingQueue);
  const acceptRef = useLatest(accept);
  const declineRef = useLatest(decline);
  useEffect(() => {
    if (!isDesktopTauri) return;
    let unsubAccept: undefined | (() => void);
    let unsubDecline: undefined | (() => void);
    let cancelled = false;
    listen<{ ring_id: string }>('ring:accept', e => {
      const ring = queueRef.current.find(r => r.ring_id === e.payload.ring_id);
      if (ring) void acceptRef.current(ring);
    }).then(fn => { if (cancelled) fn(); else unsubAccept = fn; });
    listen<{ ring_id: string }>('ring:decline', e => {
      const ring = queueRef.current.find(r => r.ring_id === e.payload.ring_id);
      if (ring) void declineRef.current(ring);
    }).then(fn => { if (cancelled) fn(); else unsubDecline = fn; });
    return () => { cancelled = true; unsubAccept?.(); unsubDecline?.(); };
  }, [isDesktopTauri]);

  const receiveCallRing = useCallback((payload: { vc: string; ring_id: string; caller: RingCaller; started_at_ms: number }) => {
    dispatch({ type: 'enqueue', ring: { ring_id: payload.ring_id, vc: payload.vc, caller: payload.caller, started_at_ms: payload.started_at_ms } });
  }, []);

  const receiveCallAccept = useCallback((payload: { ring_id: string; accepter_user_id?: string }) => {
    // Remote peer accepted — activeVc was set in initiate; just advance the state
    if (store.activeRingId === payload.ring_id && store.state === 'outgoing') {
      dispatch({ type: 'set', state: 'connecting' });
    }
    dispatch({ type: 'dequeue', ring_id: payload.ring_id });
  }, [store.activeRingId, store.state]);

  const receiveCallDecline = useCallback((payload: { ring_id: string; decliner_user_id?: string }) => {
    if (store.activeRingId === payload.ring_id) {
      dispatch({ type: 'set', state: 'idle', vc: null, ringId: null, outgoingTarget: null });
    }
    dispatch({ type: 'dequeue', ring_id: payload.ring_id });
  }, [store.activeRingId]);

  const receiveCallCancel = useCallback((payload: { ring_id: string }) => {
    dispatch({ type: 'dequeue', ring_id: payload.ring_id });
    if (store.activeRingId === payload.ring_id) {
      dispatch({ type: 'set', state: 'idle', vc: null, ringId: null });
    }
  }, [store.activeRingId]);

  const receiveCallTimeout = useCallback((payload: { ring_id: string }) => {
    dispatch({ type: 'dequeue', ring_id: payload.ring_id });
    if (store.activeRingId === payload.ring_id) {
      dispatch({ type: 'set', state: 'idle', vc: null, ringId: null, outgoingTarget: null });
    }
  }, [store.activeRingId]);

  const value: CallContextValue = {
    state: store.state,
    activeVc: store.activeVc,
    activeRingId: store.activeRingId,
    incomingQueue: store.incomingQueue,
    floatingMode: store.floatingMode,
    outgoingTarget: store.outgoingTarget,
    isDesktopTauri,
    mainFocused,
    initiate,
    accept,
    decline,
    cancel,
    notifyConnected,
    notifyDisconnected,
    setFloatingMode,
    receiveCallRing,
    receiveCallAccept,
    receiveCallDecline,
    receiveCallCancel,
    receiveCallTimeout,
  };

  return <CallContext.Provider value={value}>{children}</CallContext.Provider>;
};

export function useCall(): CallContextValue {
  const ctx = useContext(CallContext);
  if (!ctx) throw new Error('useCall must be used within a CallProvider');
  return ctx;
}
