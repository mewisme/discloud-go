import type { NextRequest } from "next/server";

import { proxyRuntime, proxyToAPI } from "@/lib/api-proxy";

export const { runtime, dynamic } = proxyRuntime;

type Ctx = { params: Promise<{ path: string[] }> };

async function handle(req: NextRequest, ctx: Ctx) {
  const { path } = await ctx.params;
  return proxyToAPI(req, `/api/${path.map(encodeURIComponent).join("/")}`);
}

export const GET = handle;
export const HEAD = handle;
export const POST = handle;
export const PUT = handle;
export const PATCH = handle;
export const DELETE = handle;
export const OPTIONS = handle;
