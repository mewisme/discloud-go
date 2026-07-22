import { proxyInstall } from "@/lib/install-proxy";

export const dynamic = "force-dynamic";

export function GET() {
  return proxyInstall("/install.ps1");
}
