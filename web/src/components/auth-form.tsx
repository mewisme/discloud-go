"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { ApiError, signIn, signUp } from "@/lib/api";
import { setAuthUser } from "@/lib/auth";

export function AuthForm({ mode }: { mode: "signin" | "signup" }) {
  const router = useRouter();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);

  const isSignUp = mode === "signup";

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (password.length < 8) {
      toast.error("Password must be at least 8 characters");
      return;
    }
    setBusy(true);
    try {
      const user = isSignUp
        ? await signUp(username, password)
        : await signIn(username, password);
      setAuthUser(user);
      toast.success(isSignUp ? "Account created" : "Signed in");
      router.push("/files");
      router.refresh();
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.message
          : err instanceof Error
            ? err.message
            : "Request failed";
      toast.error(msg);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card className="mx-auto w-full max-w-md">
      <CardHeader>
        <CardTitle>{isSignUp ? "Create account" : "Sign in"}</CardTitle>
        <CardDescription>
          {isSignUp
            ? "Username and password. First account on the server becomes admin."
            : "Use the username and password you signed up with."}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form className="flex flex-col gap-4" onSubmit={onSubmit}>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="auth-username">Username</FieldLabel>
              <Input
                id="auth-username"
                type="text"
                autoComplete="username"
                required
                minLength={3}
                maxLength={32}
                pattern="[a-zA-Z0-9][a-zA-Z0-9_-]*"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                disabled={busy}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="auth-password">Password</FieldLabel>
              <Input
                id="auth-password"
                type="password"
                autoComplete={isSignUp ? "new-password" : "current-password"}
                required
                minLength={8}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                disabled={busy}
              />
            </Field>
          </FieldGroup>
          <Button type="submit" disabled={busy}>
            {busy ? <Spinner data-icon="inline-start" /> : null}
            {busy
              ? isSignUp
                ? "Creating…"
                : "Signing in…"
              : isSignUp
                ? "Sign up"
                : "Sign in"}
          </Button>
          <p className="text-center text-sm text-muted-foreground">
            {isSignUp ? (
              <>
                Already have an account?{" "}
                <Link href="/signin" className="underline-offset-2 hover:underline">
                  Sign in
                </Link>
              </>
            ) : (
              <>
                Need an account?{" "}
                <Link href="/signup" className="underline-offset-2 hover:underline">
                  Sign up
                </Link>
              </>
            )}
          </p>
        </form>
      </CardContent>
    </Card>
  );
}
