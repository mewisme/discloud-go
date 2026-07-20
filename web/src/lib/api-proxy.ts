import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";

import { API_URL } from "@/lib/api";

/** Hop-by-hop / Next-only headers that must not be forwarded upstream. */
const DROP_REQ = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailers",
  "transfer-encoding",
  "upgrade",
  "host",
  "content-length",
]);

const DROP_RES = new Set([
  "connection",
  "keep-alive",
  "transfer-encoding",
  "content-encoding",
]);

/**
 * Stream a request to the private Go API and stream the response back.
 * Used instead of next.config rewrites so large uploads/downloads aren't
 * truncated by the rewrite proxy body buffer (and CF sees a real origin route).
 */
export async function proxyToAPI(
  req: NextRequest,
  upstreamPath: string,
): Promise<NextResponse> {
  const target = `${API_URL}${upstreamPath}${req.nextUrl.search}`;
  const headers = new Headers();
  req.headers.forEach((value, key) => {
    if (!DROP_REQ.has(key.toLowerCase())) headers.set(key, value);
  });

  const init: RequestInit & { duplex?: "half" } = {
    method: req.method,
    headers,
    redirect: "manual",
  };
  if (req.method !== "GET" && req.method !== "HEAD" && req.body) {
    init.body = req.body;
    init.duplex = "half";
  }

  let upstream: Response;
  try {
    upstream = await fetch(target, init);
  } catch (err) {
    const message = err instanceof Error ? err.message : "upstream unreachable";
    return NextResponse.json(
      { error: `API proxy failed: ${message}` },
      { status: 502 },
    );
  }

  const out = new Headers();
  upstream.headers.forEach((value, key) => {
    if (!DROP_RES.has(key.toLowerCase())) out.set(key, value);
  });

  return new NextResponse(upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers: out,
  });
}

export const proxyRuntime = {
  runtime: "nodejs" as const,
  dynamic: "force-dynamic" as const,
};
