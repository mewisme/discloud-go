import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

/** Private-link pages: strip Referer when `?token=` is present. */
export function proxy(request: NextRequest) {
  if (!request.nextUrl.searchParams.has("token")) {
    return NextResponse.next();
  }
  const res = NextResponse.next();
  res.headers.set("Referrer-Policy", "no-referrer");
  return res;
}

export const config = {
  matcher: ["/i/:path*"],
};
