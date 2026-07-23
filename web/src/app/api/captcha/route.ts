import { NextResponse } from "next/server";

export const dynamic = "force-dynamic";

/** Public Turnstile site key for the upload widget (not a secret). */
export async function GET() {
  const siteKey = process.env.CAPTCHA_KEY?.trim() ?? "";
  return NextResponse.json(
    siteKey
      ? { enabled: true, siteKey, provider: "turnstile" as const }
      : { enabled: false, siteKey: "", provider: null },
  );
}
