"use client";

import Link from "next/link";
import { useEffect, useSyncExternalStore } from "react";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { buttonVariants } from "@/components/ui/button";
import {
  ensureAuth,
  getAuthServerSnapshot,
  getAuthSnapshot,
  subscribeAuth,
} from "@/lib/auth";
import { cn } from "@/lib/utils";

export function userInitial(username: string): string {
  const ch = username.trim().charAt(0);
  return ch ? ch.toUpperCase() : "?";
}

export function AuthHeader() {
  const user = useSyncExternalStore(
    subscribeAuth,
    getAuthSnapshot,
    getAuthServerSnapshot,
  );

  useEffect(() => {
    void ensureAuth();
  }, []);

  if (user === undefined) {
    return <div className="size-8 animate-pulse rounded-full bg-muted" aria-hidden />;
  }

  if (!user) {
    return (
      <div className="flex shrink-0 items-center gap-1">
        <Link
          href="/signin"
          className={cn(buttonVariants({ variant: "ghost", size: "sm" }), "px-2 sm:px-3")}
        >
          Sign in
        </Link>
        <Link
          href="/signup"
          className={cn(
            buttonVariants({ variant: "secondary", size: "sm" }),
            "px-2 sm:px-3",
          )}
        >
          Sign up
        </Link>
      </div>
    );
  }

  return (
    <Link
      href="/me"
      title={user.username}
      aria-label={`Account for ${user.username}`}
      className="shrink-0 rounded-full outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
    >
      <Avatar size="sm">
        <AvatarFallback>{userInitial(user.username)}</AvatarFallback>
      </Avatar>
    </Link>
  );
}
