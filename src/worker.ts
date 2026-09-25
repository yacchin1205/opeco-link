import { HttpError, IDENTIFIER, expectKeys, json, readObject, stringField } from "./http";
import { DeviceRegistry } from "./device";
import { DeviceRequest } from "./device-request";
import { DeviceGroup } from "./group";
import { Session } from "./session";

interface Env {
  SESSIONS: DurableObjectNamespace<Session>;
  GROUPS: DurableObjectNamespace<DeviceGroup>;
  DEVICES: DurableObjectNamespace<DeviceRegistry>;
  DEVICE_REQUESTS: DurableObjectNamespace<DeviceRequest>;
  ASSETS: Fetcher;
  ATTACHMENTS: R2Bucket;
  SESSION_CREATE_LIMIT: RateLimit;
  PAIRING_LIMIT: RateLimit;
  REGISTRATION_LIMIT: RateLimit;
  EVENT_LIMIT: RateLimit;
  RESPONSE_LIMIT: RateLimit;
}

export { DeviceGroup, DeviceRegistry, DeviceRequest, Session };

// The demo page at this origin drives the API from a browser as a session
// creator, so it needs CORS; every other client is same-origin or non-browser.
const CORS_ORIGINS = new Set(["https://demo.opeco.link"]);

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const origin = request.headers.get("origin");
    const url = new URL(request.url);
    if (origin === null || !CORS_ORIGINS.has(origin) || !url.pathname.startsWith("/api/")) {
      return relay.fetch(request, env);
    }
    if (request.method === "OPTIONS") {
      return new Response(null, {
        status: 204,
        headers: {
          "access-control-allow-origin": origin,
          "access-control-allow-methods": "GET, POST, PUT, DELETE",
          "access-control-allow-headers": "authorization, content-type",
          "access-control-max-age": "86400",
          vary: "origin",
        },
      });
    }
    const response = await relay.fetch(request, env);
    const headers = new Headers(response.headers);
    headers.set("access-control-allow-origin", origin);
    headers.append("vary", "origin");
    return new Response(response.body, { status: response.status, headers });
  },
};

const relay = {
  async fetch(request: Request, env: Env): Promise<Response> {
    try {
      const url = new URL(request.url);
      const requestHost = request.headers.get("host")?.split(":", 1)[0];
      const miniflareLoopback = url.hostname === "opeco.link"
        && request.headers.get("mf-original-hostname") === "opeco.link"
        && request.headers.get("cf-connecting-ip") === "127.0.0.1";
      const localRequest = miniflareLoopback
        || [url.hostname, requestHost].some((host) => host === "localhost" || host === "127.0.0.1");
      if (url.protocol === "http:" && !localRequest) {
        url.protocol = "https:";
        return Response.redirect(url.toString(), 301);
      }
      // Local development traffic (wrangler dev, the browser suite, the Go
      // integration test) shares one loopback address and is not rate limited.
      const throttle = async (limiter: RateLimit, key: string) => {
        if (!localRequest) await limit(limiter, key);
      };
      if (!url.pathname.startsWith("/api/")) {
        return env.ASSETS.fetch(request);
      }
      if (request.method === "GET" && url.pathname === "/api/health") {
        return json({ ok: true });
      }
      if (request.method === "POST" && url.pathname === "/api/devices") {
        await throttle(env.REGISTRATION_LIMIT, clientAddress(request));
        return deviceRegistry(env).fetch(publicRequest(new URL("https://devices.internal/devices"), request));
      }
      const devicePushMatch = /^\/api\/devices\/([^/]+)\/push$/.exec(url.pathname);
      if (request.method === "PUT" && devicePushMatch !== null) {
        const deviceId = stringField({ deviceId: devicePushMatch[1] }, "deviceId", IDENTIFIER, 64);
        return deviceRegistry(env).fetch(publicRequest(
          new URL(`https://devices.internal/devices/${deviceId}/push`),
          request,
        ));
      }
      if (request.method === "POST" && url.pathname === "/api/device-requests") {
        await throttle(env.REGISTRATION_LIMIT, clientAddress(request));
        return deviceRegistry(env).fetch(publicRequest(
          new URL("https://devices.internal/device-requests"),
          request,
        ));
      }
      const deviceRequestMatch = /^\/api\/device-requests\/([^/]+)$/.exec(url.pathname);
      if (request.method === "GET" && deviceRequestMatch !== null) {
        const requestId = stringField({ requestId: deviceRequestMatch[1] }, "requestId", IDENTIFIER, 64);
        const internalUrl = new URL("https://device-request.internal/");
        internalUrl.search = url.search;
        return deviceRequestStub(env, requestId).fetch(publicRequest(internalUrl, request));
      }
      if (request.method === "POST" && url.pathname === "/api/sessions") {
        await throttle(env.SESSION_CREATE_LIMIT, clientAddress(request));
        const body = await readObject(request);
        expectKeys(body, ["sessionId", "sessionTokenHash", "creatorPublicKey", "pairing"], ["protocolVersion"]);
        const sessionId = stringField(body, "sessionId", IDENTIFIER, 64);
        return sessionStub(env, sessionId).fetch(
          forwardedRequest("/create", request, JSON.stringify(body)),
        );
      }

      if (request.method === "POST" && url.pathname === "/api/groups") {
        await throttle(env.REGISTRATION_LIMIT, clientAddress(request));
        const body = await readObject(request);
        const groupId = stringField(body, "groupId", IDENTIFIER, 64);
        return groupStub(env, groupId).fetch(
          forwardedRequest("/create", request, JSON.stringify(body)),
        );
      }

      const groupDeviceRequestMatch = /^\/api\/groups\/([^/]+)\/device-requests\/([^/]+)(\/approve)?$/.exec(url.pathname);
      if (groupDeviceRequestMatch !== null) {
        const groupId = stringField({ groupId: groupDeviceRequestMatch[1] }, "groupId", IDENTIFIER, 64);
        const requestId = stringField({ requestId: groupDeviceRequestMatch[2] }, "requestId", IDENTIFIER, 64);
        const suffix = groupDeviceRequestMatch[3] ?? "";
        if (request.method === "POST" && suffix === "/approve") await throttle(env.REGISTRATION_LIMIT, clientAddress(request));
        const internalUrl = new URL(`https://device-request.internal/groups/${groupId}${suffix}`);
        internalUrl.search = url.search;
        return deviceRequestStub(env, requestId).fetch(publicRequest(internalUrl, request));
      }

      const groupMatch = /^\/api\/groups\/([^/]+)(\/.*)?$/.exec(url.pathname);
      if (groupMatch !== null) {
        const groupId = stringField({ groupId: groupMatch[1] }, "groupId", IDENTIFIER, 64);
        const internalUrl = new URL(`https://group.internal${groupMatch[2] ?? "/"}`);
        internalUrl.search = url.search;
        return groupStub(env, groupId).fetch(publicRequest(internalUrl, request));
      }

      const match = /^\/api\/sessions\/([^/]+)(\/.*)?$/.exec(url.pathname);
      if (match === null) {
        throw new HttpError(404, "not_found", "Endpoint not found");
      }
      const sessionId = stringField({ sessionId: match[1] }, "sessionId", IDENTIFIER, 64);
      const operation = match[2] ?? "/";
      if (request.method === "POST" && operation === "/pairings") await throttle(env.PAIRING_LIMIT, clientAddress(request));
      if (request.method === "POST" && operation === "/events") await throttle(env.EVENT_LIMIT, sessionId);
      if ((request.method === "POST" && (operation === "/responses" || operation === "/attachments"))
        || (request.method === "PUT" && operation.startsWith("/attachments/"))) {
        await throttle(env.RESPONSE_LIMIT, sessionId);
      }
      const internalUrl = new URL(`https://session.internal${match[2] ?? "/"}`);
      internalUrl.search = url.search;
      return sessionStub(env, sessionId).fetch(publicRequest(internalUrl, request));
    } catch (error) {
      if (error instanceof HttpError) {
        const response = json({ error: error.code, message: error.message }, error.status);
        if (error.status === 429) response.headers.set("retry-after", "60");
        return response;
      }
      throw error;
    }
  },
};

// Cloudflare sets the client address on every request that reaches the Worker;
// a request without it is not a client request and must not share a bucket.
function clientAddress(request: Request): string {
  const address = request.headers.get("cf-connecting-ip");
  if (address === null) throw new Error("cf-connecting-ip header is missing");
  return address;
}

async function limit(limiter: RateLimit, key: string): Promise<void> {
  const { success } = await limiter.limit({ key });
  if (!success) throw new HttpError(429, "rate_limited", "Too many requests for this operation; retry after a minute");
}

function sessionStub(env: Env, sessionId: string): DurableObjectStub<Session> {
  return env.SESSIONS.get(env.SESSIONS.idFromName(sessionId));
}

function groupStub(env: Env, groupId: string): DurableObjectStub<DeviceGroup> {
  return env.GROUPS.get(env.GROUPS.idFromName(groupId));
}

function deviceRegistry(env: Env): DurableObjectStub<DeviceRegistry> {
  return env.DEVICES.get(env.DEVICES.idFromName("registry"));
}

function deviceRequestStub(env: Env, requestId: string): DurableObjectStub<DeviceRequest> {
  return env.DEVICE_REQUESTS.get(env.DEVICE_REQUESTS.idFromName(requestId));
}

function forwardedRequest(path: string, original: Request, body: string): Request {
  const headers = new Headers(original.headers);
  return new Request(`https://session.internal${path}`, {
    method: original.method,
    headers,
    body,
  });
}

function publicRequest(url: URL, original: Request): Request {
  const headers = new Headers(original.headers);
  return new Request(url, { method: original.method, headers, body: original.body });
}
