export interface ForgeConfig {
  endpoint: string;
}

export class ForgeCore {
  private token: string | null = null;
  readonly endpoint: string;

  constructor(config: ForgeConfig) {
    // Strip trailing slashes
    this.endpoint = config.endpoint.replace(/\/$/, "");
  }

  setToken(token: string) {
    this.token = token;
  }

  getToken(): string | null {
    return this.token;
  }

  async fetch(path: string, options: RequestInit = {}): Promise<Response> {
    const headers = new Headers(options.headers || {});
    if (this.token) {
      headers.set("Authorization", `Bearer ${this.token}`);
    }

    // Default to JSON if sending a body
    if (options.body && typeof options.body === "string" && !headers.has("Content-Type")) {
      headers.set("Content-Type", "application/json");
    }

    const response = await fetch(`${this.endpoint}${path}`, {
      ...options,
      headers,
    });

    return response;
  }
}
