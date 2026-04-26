import React, { createContext, useContext, useReducer, useCallback, useEffect, useRef } from 'react';
import { ringDm, acceptCall, declineCall, cancelCall, type ActiveRing, type RingCaller } from '../api/calls';

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
