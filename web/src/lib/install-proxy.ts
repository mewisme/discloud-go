/** Upstream API origin for server-side proxy (install scripts, etc.). */
export function apiUpstream(): string {
  return (
    process.env.API_UPSTREAM ||
    process.env.API_URL ||
    "http://localhost:8080"
  ).replace(/\/$/, "");
}

/** Proxy GET to the API install script, preserving Content-Type. */
export async function proxyInstall(
  path: "/install.sh" | "/install.ps1",
): Promise<Response> {
  const res = await fetch(`${apiUpstream()}${path}`, { cache: "no-store" });
  const body = await res.arrayBuffer();
  const contentType =
    res.headers.get("Content-Type") ||
    (path.endsWith(".sh")
      ? "text/x-shellscript; charset=utf-8"
      : "text/plain; charset=utf-8");
  return new Response(body, {
    status: res.status,
    headers: {
      "Content-Type": contentType,
      "Cache-Control": "no-store",
    },
  });
}
