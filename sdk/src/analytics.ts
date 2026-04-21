import { ForgeCore } from "./forge";

export class AnalyticsModule {
  constructor(private core: ForgeCore) {}

  async track(name: string, properties: Record<string, any> = {}): Promise<void> {
    const res = await this.core.fetch(`/analytics/track`, {
      method: "POST",
      body: JSON.stringify({ name, properties })
    });
    if (!res.ok) {
      // We don't necessarily want to throw on track failures in production apps,
      // but logging it is useful
      console.warn("Forge Analytics failed to track event:", name);
    }
  }
}
