import { ForgeCore } from "./forge";

export class StorageModule {
  constructor(private core: ForgeCore) {}

  async upload(bucket: string, file: File, key?: string): Promise<{ url: string, key: string }> {
    const formData = new FormData();
    formData.append("file", file);
    if (key) formData.append("key", key);

    const res = await this.core.fetch(`/storage/${bucket}`, {
      method: "POST",
      body: formData,
      // Let the browser set the Content-Type to multipart/form-data with boundary
      headers: { "Content-Type": "" }
    });

    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || "Failed to upload file");
    }

    return res.json();
  }

  async getUrl(bucket: string, key: string): Promise<string> {
    return `${this.core.endpoint}/storage/${bucket}/${key}`;
  }
}
