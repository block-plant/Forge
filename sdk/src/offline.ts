/**
 * Forge SDK — Offline Support
 *
 * Provides an IndexedDB-backed write queue that stores failed or pending
 * mutations while the client is offline, then replays them in FIFO order
 * when the connection is restored.
 *
 * Usage:
 *   const queue = new OfflineQueue('forge-offline-queue');
 *   queue.enqueue({ type: 'set', collection: 'posts', id: '1', data: { ... } });
 *   queue.flush(async (op) => { await forge.db.set(op.collection, op.id, op.data); });
 */

/** A single pending write operation. */
export interface PendingOperation {
  id: string;
  type: 'set' | 'update' | 'delete' | 'create';
  collection: string;
  docId?: string;
  data?: Record<string, any>;
  timestamp: number;
  retries: number;
}

/** Options for the OfflineQueue. */
export interface OfflineQueueOptions {
  /** Maximum number of retries per operation before discarding. Default: 5 */
  maxRetries?: number;
  /** Delay between flush attempts in ms. Default: 2000 */
  retryDelayMs?: number;
}

/**
 * OfflineQueue is an IndexedDB-backed persistent write queue.
 * It survives page reloads and browser crashes.
 */
export class OfflineQueue {
  private dbName: string;
  private storeName = 'pending_ops';
  private db: IDBDatabase | null = null;
  private maxRetries: number;
  private retryDelayMs: number;
  private flushTimer: ReturnType<typeof setInterval> | null = null;

  constructor(dbName = 'forge-offline-queue', opts: OfflineQueueOptions = {}) {
    this.dbName = dbName;
    this.maxRetries = opts.maxRetries ?? 5;
    this.retryDelayMs = opts.retryDelayMs ?? 2000;
  }

  /** Open (or create) the IndexedDB store. Must be called before using the queue. */
  async open(): Promise<void> {
    return new Promise((resolve, reject) => {
      const req = indexedDB.open(this.dbName, 1);
      req.onupgradeneeded = (event) => {
        const db = (event.target as IDBOpenDBRequest).result;
        if (!db.objectStoreNames.contains(this.storeName)) {
          const store = db.createObjectStore(this.storeName, { keyPath: 'id' });
          store.createIndex('timestamp', 'timestamp', { unique: false });
        }
      };
      req.onsuccess = (event) => {
        this.db = (event.target as IDBOpenDBRequest).result;
        resolve();
      };
      req.onerror = () => reject(req.error);
    });
  }

  /** Add a pending operation to the queue. */
  async enqueue(op: Omit<PendingOperation, 'id' | 'timestamp' | 'retries'>): Promise<void> {
    const record: PendingOperation = {
      ...op,
      id: crypto.randomUUID(),
      timestamp: Date.now(),
      retries: 0,
    };
    await this.put(record);
  }

  /** Return all pending operations ordered by timestamp (FIFO). */
  async pending(): Promise<PendingOperation[]> {
    return new Promise((resolve, reject) => {
      const tx = this.db!.transaction(this.storeName, 'readonly');
      const store = tx.objectStore(this.storeName);
      const index = store.index('timestamp');
      const req = index.getAll();
      req.onsuccess = () => resolve(req.result as PendingOperation[]);
      req.onerror = () => reject(req.error);
    });
  }

  /** Count of pending operations. */
  async count(): Promise<number> {
    return new Promise((resolve, reject) => {
      const tx = this.db!.transaction(this.storeName, 'readonly');
      const req = tx.objectStore(this.storeName).count();
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error);
    });
  }

  /**
   * Flush the queue by calling `handler` for each operation in order.
   * On success, the operation is dequeued.
   * On failure, retries++ — if maxRetries exceeded, the operation is dropped.
   */
  async flush(handler: (op: PendingOperation) => Promise<void>): Promise<void> {
    const ops = await this.pending();
    for (const op of ops) {
      try {
        await handler(op);
        await this.remove(op.id);
      } catch {
        if (op.retries >= this.maxRetries) {
          await this.remove(op.id); // drop after too many retries
        } else {
          op.retries++;
          await this.put(op); // update retry count
        }
      }
    }
  }

  /**
   * Start auto-flushing when online. Flushes immediately on reconnect
   * and periodically checks for queued items.
   */
  startAutoFlush(handler: (op: PendingOperation) => Promise<void>): void {
    if (typeof window === 'undefined') return;

    const tryFlush = () => {
      if (navigator.onLine) this.flush(handler).catch(() => {});
    };

    window.addEventListener('online', tryFlush);
    this.flushTimer = setInterval(tryFlush, this.retryDelayMs);
  }

  /** Stop auto-flushing. */
  stopAutoFlush(): void {
    if (this.flushTimer) {
      clearInterval(this.flushTimer);
      this.flushTimer = null;
    }
  }

  /** Clear all pending operations. */
  async clear(): Promise<void> {
    return new Promise((resolve, reject) => {
      const tx = this.db!.transaction(this.storeName, 'readwrite');
      const req = tx.objectStore(this.storeName).clear();
      req.onsuccess = () => resolve();
      req.onerror = () => reject(req.error);
    });
  }

  // ── Private ──

  private async put(op: PendingOperation): Promise<void> {
    return new Promise((resolve, reject) => {
      const tx = this.db!.transaction(this.storeName, 'readwrite');
      const req = tx.objectStore(this.storeName).put(op);
      req.onsuccess = () => resolve();
      req.onerror = () => reject(req.error);
    });
  }

  private async remove(id: string): Promise<void> {
    return new Promise((resolve, reject) => {
      const tx = this.db!.transaction(this.storeName, 'readwrite');
      const req = tx.objectStore(this.storeName).delete(id);
      req.onsuccess = () => resolve();
      req.onerror = () => reject(req.error);
    });
  }
}
