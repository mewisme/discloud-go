"use client";

import Link from "next/link";
import { useEffect, useSyncExternalStore } from "react";
import { toast } from "sonner";

import { Button, buttonVariants } from "@/components/ui/button";
import {
  ensureAuth,
  getAuthServerSnapshot,
  getAuthSnapshot,
  signOutAndClear,
  subscribeAuth,
} from "@/lib/auth";
import { cn } from "@/lib/utils";

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
    return <div className="h-8 w-24 animate-pulse rounded-md bg-muted" aria-hidden />;
  }

  if (!user) {
    return (
      <div className="flex items-center gap-2">
        <Link
          href="/signin"
          className={cn(buttonVariants({ variant: "ghost", size: "sm" }))}
        >
          Sign in
        </Link>
        <Link
          href="/signup"
          className={cn(buttonVariants({ variant: "secondary", size: "sm" }))}
        >
          Sign up
        </Link>
      </div>
    );
  }

  return (
    <div className="flex items-center gap-2">
      <Link
        href="/files"
        className={cn(
          buttonVariants({ variant: "ghost", size: "sm" }),
          "hidden sm:inline-flex",
        )}
      >
        My files
      </Link>
      <span
        className="max-w-[10rem] truncate text-xs text-muted-foreground"
        title={user.email}
      >
        {user.email}
      </span>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={async () => {
          try {
            await signOutAndClear();
            toast.success("Signed out");
          } catch {
            toast.error("Could not sign out");
          }
        }}
      >
        Sign out
      </Button>
    </div>
  );
}
