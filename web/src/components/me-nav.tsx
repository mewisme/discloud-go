"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { cn } from "@/lib/utils";

const NAV: { href: string; label: string; exact?: boolean }[] = [
  { href: "/me", label: "Account", exact: true },
  { href: "/me/tokens", label: "API tokens" },
  { href: "/me/security", label: "Security" },
  { href: "/me/preferences", label: "Preferences" },
];

export function MeNav() {
  const pathname = usePathname();

  return (
    <nav
      aria-label="Account sections"
      className="flex flex-wrap gap-1 border-b border-border/60 pb-px"
    >
      {NAV.map((item) => {
        const active = item.exact
          ? pathname === item.href
          : pathname === item.href || pathname.startsWith(`${item.href}/`);
        return (
          <Link
            key={item.href}
            href={item.href}
            className={cn(
              "-mb-px border-b-2 px-3 py-2 text-sm transition-colors",
              active
                ? "border-foreground font-medium text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )}
            aria-current={active ? "page" : undefined}
          >
            {item.label}
          </Link>
        );
      })}
    </nav>
  );
}
