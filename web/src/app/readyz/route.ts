import type { NextRequest } from "next/server";

import { proxyRuntime, proxyToAPI } from "@/lib/api-proxy";

export const { runtime, dynamic } = proxyRuntime;

export function GET(req: NextRequest) {
  return proxyToAPI(req, "/readyz");
}

export function HEAD(req: NextRequest) {
  return proxyToAPI(req, "/readyz");
}
