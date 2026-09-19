import { resolve } from "node:path";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
import { test as base, expect, type Page } from "@playwright/test";

type AgentSession = {
  pairingURL: string;
  call: (name: string, args?: Record<string, unknown>) => Promise<Record<string, unknown>>;
};

export const test = base.extend<{
  createSession: (title: string, color?: string) => Promise<AgentSession>;
}>({
  createSession: async ({ baseURL }, use) => {
    const clients: Client[] = [];
    const sessions: AgentSession[] = [];
    try {
      await use(async (title, color = "#d0dbf2") => {
        const client = new Client({ name: "opeco-web-e2e", version: "1.0.0" });
        clients.push(client);
        await client.connect(new StdioClientTransport({
          command: resolve(".wrangler/e2e/opeco"),
          args: ["--base-url", baseURL!, "mcp"],
          stderr: "inherit",
        }));
        const call = async (name: string, args: Record<string, unknown>) => {
          const result = await client.callTool({ name, arguments: args });
          expect(result.isError, JSON.stringify(result.content)).not.toBe(true);
          expect(result.structuredContent).toBeDefined();
          return result.structuredContent as Record<string, unknown>;
        };
        const created = await call("session_create", { title, color });
        expect(created).toMatchObject({ reused: false, device_group_count: 0 });
        expect(typeof created.pairing_url).toBe("string");
        const session = {
          pairingURL: created.pairing_url as string,
          call: (name: string, args = {}) => call(name, { ...args, session_id: created.session_id }),
        };
        sessions.push(session);
        return session;
      });
    } finally {
      try {
        await Promise.all(sessions.map((session) => session.call("session_close")));
      } finally {
        await Promise.all(clients.map((client) => client.close()));
      }
    }
  },
});

export { expect };

export async function screenshot(page: Page, name: string) {
  const path = test.info().outputPath(`${name}.png`);
  await page.screenshot({ path, fullPage: true });
  await test.info().attach(name, { path, contentType: "image/png" });
}
