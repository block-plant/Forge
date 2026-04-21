import { ForgeCore } from "./forge";
import { AuthModule } from "./auth";
import { DatabaseModule } from "./db";
import { StorageModule } from "./storage";
import { FunctionsModule } from "./functions";
import { AnalyticsModule } from "./analytics";
import { RealtimeManager } from "./realtime";

// Re-export all types
export * from "./types";
export { ForgeConfig } from "./forge";

export class ForgeClient {
  public core: ForgeCore;
  public auth: AuthModule;
  public db: DatabaseModule;
  public storage: StorageModule;
  public functions: FunctionsModule;
  public analytics: AnalyticsModule;
  public realtime: RealtimeManager;

  constructor(config: { endpoint: string }) {
    this.core = new ForgeCore(config);
    this.auth = new AuthModule(this.core);
    this.db = new DatabaseModule(this.core);
    this.storage = new StorageModule(this.core);
    this.functions = new FunctionsModule(this.core);
    this.analytics = new AnalyticsModule(this.core);
    this.realtime = new RealtimeManager(this.core);
  }
}

export function initializeApp(config: { endpoint: string }): ForgeClient {
  return new ForgeClient(config);
}
