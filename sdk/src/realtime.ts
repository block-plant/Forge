import { ForgeCore } from "./forge";
import { ConnectionState, RealtimeMessage, UnsubscribeFn } from "./types";

/**
 * RealtimeManager handles the WebSocket connection lifecycle
 * with automatic reconnection, exponential backoff, heartbeat
 * ping/pong, and subscription persistence across reconnects.
 */
export class RealtimeManager {
  private ws: WebSocket | null = null;
  private url: string;
  private core: ForgeCore;
  private state: ConnectionState = "disconnected";

  // Subscriptions to restore on reconnect
  private subscriptions: Map<string, { channel: string; callback: (data: any) => void }> = new Map();
  private subIdCounter = 0;

  // Reconnection backoff
  private reconnectAttempt = 0;
  private maxReconnectAttempt = 10;
  private baseDelay = 500;  // ms
  private maxDelay = 30000; // ms
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  // Heartbeat
  private pingInterval: ReturnType<typeof setInterval> | null = null;
  private pingIntervalMs = 30000;

  // State change listeners
  private stateListeners: ((state: ConnectionState) => void)[] = [];

  constructor(core: ForgeCore) {
    this.core = core;
    const parsed = new URL(core.endpoint);
    const protocol = parsed.protocol === "https:" ? "wss:" : "ws:";
    this.url = `${protocol}//${parsed.host}/realtime`;
  }

  /** Connect to the WebSocket server. */
  connect(): void {
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return;
    }

    this.setState("connecting");

    this.ws = new WebSocket(this.url);

    this.ws.onopen = () => {
      this.setState("connected");
      this.reconnectAttempt = 0;

      // Authenticate
      const token = this.core.getToken();
      if (token) {
        this.send({ type: "auth", payload: { token } });
      }

      // Restore subscriptions
      for (const [id, sub] of this.subscriptions) {
        this.send({ type: "subscribe", payload: { channel: sub.channel }, id });
      }

      // Start heartbeat
      this.startHeartbeat();
    };

    this.ws.onmessage = (event) => {
      try {
        const msg: RealtimeMessage = JSON.parse(event.data);

        if (msg.type === "ping") {
          this.send({ type: "pong" });
          return;
        }

        if (msg.type === "message" && msg.channel) {
          // Route to all subscriptions for this channel
          for (const [, sub] of this.subscriptions) {
            if (sub.channel === msg.channel) {
              sub.callback(msg.payload);
            }
          }
        }
      } catch {
        // ignore parse errors
      }
    };

    this.ws.onclose = () => {
      this.stopHeartbeat();
      if (this.state !== "disconnected") {
        this.setState("reconnecting");
        this.scheduleReconnect();
      }
    };

    this.ws.onerror = () => {
      // onclose will fire after onerror
    };
  }

  /** Subscribe to a channel. Returns an unsubscribe function. */
  subscribe(channel: string, callback: (data: any) => void): UnsubscribeFn {
    const id = `sub_${++this.subIdCounter}`;

    this.subscriptions.set(id, { channel, callback });

    // If connected, subscribe immediately
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.send({ type: "subscribe", payload: { channel }, id });
    }

    return () => {
      this.subscriptions.delete(id);
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.send({ type: "unsubscribe", id });
      }
    };
  }

  /** Register a connection state change listener. */
  onStateChange(listener: (state: ConnectionState) => void): UnsubscribeFn {
    this.stateListeners.push(listener);
    return () => {
      this.stateListeners = this.stateListeners.filter((l) => l !== listener);
    };
  }

  /** Get current connection state. */
  getState(): ConnectionState {
    return this.state;
  }

  /** Disconnect and stop all reconnection attempts. */
  disconnect(): void {
    this.setState("disconnected");
    this.stopHeartbeat();

    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }

    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  // ---- Private ----

  private send(msg: Record<string, any>): void {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg));
    }
  }

  private setState(state: ConnectionState): void {
    this.state = state;
    for (const listener of this.stateListeners) {
      try { listener(state); } catch { /* ignore */ }
    }
  }

  private scheduleReconnect(): void {
    if (this.reconnectAttempt >= this.maxReconnectAttempt) {
      this.setState("disconnected");
      return;
    }

    // Exponential backoff with jitter
    const delay = Math.min(
      this.baseDelay * Math.pow(2, this.reconnectAttempt) + Math.random() * 500,
      this.maxDelay
    );

    this.reconnectAttempt++;
    this.reconnectTimer = setTimeout(() => this.connect(), delay);
  }

  private startHeartbeat(): void {
    this.stopHeartbeat();
    this.pingInterval = setInterval(() => {
      this.send({ type: "ping" });
    }, this.pingIntervalMs);
  }

  private stopHeartbeat(): void {
    if (this.pingInterval) {
      clearInterval(this.pingInterval);
      this.pingInterval = null;
    }
  }
}
