import { ForgeCore } from "./forge";

export class AuthModule {
  constructor(private core: ForgeCore) {}

  async signup(email: string, password: string):Promise<any> {
    const res = await this.core.fetch("/auth/signup", {
      method: "POST",
      body: JSON.stringify({ email, password })
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || "Signup failed");
    this.core.setToken(data.token);
    return data;
  }

  async login(email: string, password: string):Promise<any> {
    const res = await this.core.fetch("/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password })
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || "Login failed");
    this.core.setToken(data.token);
    return data;
  }

  async me():Promise<any> {
    const res = await this.core.fetch("/auth/me");
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || "Failed to fetch profile");
    return data;
  }
}
