import { useRef, useState, useEffect, useCallback } from "react";

const WS_URL = import.meta.env.VITE_WS_URL || "ws://localhost:8001/ws";

const INITIAL_DELAY = 1000;
const MAX_DELAY = 30000;

// Close codes that should not trigger reconnection
const NO_RECONNECT_CODES = [1000]; // normal close only

export function useWebSocket(token, onMessage, onTokenExpired) {
  const [status, setStatus] = useState("disconnected"); // "connecting" | "connected" | "disconnected"
  const wsRef = useRef(null);
  const reconnectTimer = useRef(null);
  const attemptRef = useRef(0);
  const tokenRef = useRef(token);
  const onMessageRef = useRef(onMessage);
  const onTokenExpiredRef = useRef(onTokenExpired);
  const connectRef = useRef(null);

  // Keep refs current without re-triggering the effect
  useEffect(() => { tokenRef.current = token; }, [token]);
  useEffect(() => { onMessageRef.current = onMessage; }, [onMessage]);
  useEffect(() => { onTokenExpiredRef.current = onTokenExpired; }, [onTokenExpired]);

  const clearReconnect = useCallback(() => {
    if (reconnectTimer.current) {
      clearTimeout(reconnectTimer.current);
      reconnectTimer.current = null;
    }
  }, []);

  const scheduleReconnect = useCallback(() => {
    const attempt = attemptRef.current;
    attemptRef.current = attempt + 1;

    // Exponential backoff with jitter
    const base = Math.min(INITIAL_DELAY * Math.pow(2, attempt), MAX_DELAY);
    const jitter = base * 0.5 * Math.random();
    const delay = base * 0.5 + jitter;

    reconnectTimer.current = setTimeout(() => {
      reconnectTimer.current = null;
      connectRef.current?.();
    }, delay);
  }, []);

  const connect = useCallback(() => {
    if (!tokenRef.current) return;

    clearReconnect();

    // Close existing connection if any
    if (wsRef.current) {
      wsRef.current.onclose = null;
      wsRef.current.close();
    }

    setStatus("connecting");
    const ws = new WebSocket(`${WS_URL}?token=${tokenRef.current}`);
    wsRef.current = ws;

    ws.onopen = () => {
      attemptRef.current = 0;
      setStatus("connected");
    };

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        onMessageRef.current?.(msg);
      } catch {
        // ignore unparseable messages
      }
    };

    ws.onclose = (event) => {
      wsRef.current = null;
      setStatus("disconnected");

      if (NO_RECONNECT_CODES.includes(event.code)) return;
      if (!tokenRef.current) return;

      // Token rejected — ask caller to refresh, then reconnect with new token
      if (event.code === 1008) {
        onTokenExpiredRef.current?.();
        return;
      }

      scheduleReconnect();
    };

    ws.onerror = () => {
      // onclose will fire after onerror — reconnect logic lives there
    };
  }, [clearReconnect, scheduleReconnect]);

  // Keep connectRef in sync so scheduleReconnect can call it
  useEffect(() => { connectRef.current = connect; }, [connect]);

  // Connect when token becomes available, disconnect when it's removed
  useEffect(() => {
    if (token) {
      connect();
    } else {
      clearReconnect();
      if (wsRef.current) {
        wsRef.current.onclose = null;
        wsRef.current.close(1000, "logout");
        wsRef.current = null;
      }
      setStatus("disconnected");
    }

    return () => {
      clearReconnect();
      if (wsRef.current) {
        wsRef.current.onclose = null;
        wsRef.current.close(1000, "unmount");
        wsRef.current = null;
      }
    };
  }, [token, connect, clearReconnect]);

  const sendMessage = useCallback((roomId, content, nonce) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({
        type: "send_message",
        room_id: roomId,
        content,
        nonce,
      }));
      return true;
    }
    return false;
  }, []);

  return { status, sendMessage };
}
