import { ForgeCore } from "./forge";

export class DatabaseModule {
  constructor(private core: ForgeCore) {}

  collection(name: string) {
    return new CollectionReference(this.core, name);
  }
}

export class CollectionReference {
  constructor(private core: ForgeCore, private name: string) {}

  async set(id: string, data: any): Promise<void> {
    const res = await this.core.fetch(`/db/${this.name}/${id}`, {
      method: "PUT",
      body: JSON.stringify(data)
    });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || "Failed to set document");
    }
  }

  async get(id: string): Promise<any> {
    const res = await this.core.fetch(`/db/${this.name}/${id}`);
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || "Failed to get document");
    }
    return res.json();
  }

  async onSnapshot(callback: (event: any) => void): Promise<() => void> {
    // Determine realtime URL (ws:// or wss://)
    const url = new URL(this.core.endpoint);
    const protocol = url.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${protocol}//${url.host}/realtime`;

    const ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      // Authenticate if we have a token
      const token = this.core.getToken();
      if (token) {
        ws.send(JSON.stringify({ type: "auth", payload: { token } }));
      }
      // Subscribe to the collection changes map directly into Phase 4
      ws.send(JSON.stringify({ type: "subscribe", payload: { channel: `db:${this.name}` } }));
    };

    ws.onmessage = (msg) => {
      try {
        const data = JSON.parse(msg.data);
        if (data.type === "message") {
          callback(data.payload);
        }
      } catch (e) {
        // ignore parse error
      }
    };

    // Return an unsubscribe function
    return () => ws.close();
  }
}
