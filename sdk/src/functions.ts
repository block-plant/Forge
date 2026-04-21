import { ForgeCore } from "./forge";

export class FunctionsModule {
  constructor(private core: ForgeCore) {}

  async invoke(name: string, payload: any = {}): Promise<any> {
    const res = await this.core.fetch(`/functions/invoke/${name}`, {
      method: "POST",
      body: JSON.stringify(payload)
    });

    const data = await res.json();
    if (!res.ok) {
      throw new Error(data.error || `Failed to invoke function ${name}`);
    }

    return data;
  }
}
