"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useSyncExternalStore } from "react";
import { toast } from "sonner";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button, buttonVariants } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  ensureAuth,
  getAuthServerSnapshot,
  getAuthSnapshot,
  signOutAndClear,
  subscribeAuth,
} from "@/lib/auth";
import { cn } from "@/lib/utils";

export function userInitial(username: string): string {
  const ch = username.trim().charAt(0);
  return ch ? ch.toUpperCase() : "?";
}

export function AuthHeader() {
  const router = useRouter();
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
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            className="shrink-0 rounded-full"
          />
        }
        aria-label={`Account for ${user.username}`}
        title={user.username}
      >
        <Avatar size="sm">
          <AvatarFallback>{userInitial(user.username)}</AvatarFallback>
        </Avatar>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-44">
        <DropdownMenuGroup>
          <DropdownMenuLabel>{user.username}</DropdownMenuLabel>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem
            nativeButton={false}
            closeOnClick
            render={<Link href="/me" />}
          >
            Account
          </DropdownMenuItem>
          {user.role === "admin" ? (
            <DropdownMenuItem
              nativeButton={false}
              closeOnClick
              render={<Link href="/admin" />}
            >
              Admin
            </DropdownMenuItem>
          ) : null}
          <DropdownMenuItem
            nativeButton={false}
            closeOnClick
            render={<Link href="/me/tokens" />}
          >
            API tokens
          </DropdownMenuItem>
          <DropdownMenuItem
            nativeButton={false}
            closeOnClick
            render={<Link href="/me/security" />}
          >
            Security
          </DropdownMenuItem>
          <DropdownMenuItem
            nativeButton={false}
            closeOnClick
            render={<Link href="/me/preferences" />}
          >
            Preferences
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem
            variant="destructive"
            onClick={async () => {
              try {
                await signOutAndClear();
                toast.success("Signed out");
                router.push("/");
                router.refresh();
              } catch {
                toast.error("Could not sign out");
              }
            }}
          >
            Log out
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
