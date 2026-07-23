"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";

import { CopyButton } from "@/components/copy-button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import {
  ApiError,
  createAPIToken,
  listAPITokens,
  revokeAPIToken,
  type APITokenCreated,
  type APITokenMeta,
  type APITokenScope,
} from "@/lib/api";
import { formatDate } from "@/lib/format";

const ALL_SCOPES: { id: APITokenScope; label: string; hint: string }[] = [
  { id: "upload", label: "upload", hint: "Chunks and upload sessions" },
  { id: "read", label: "read", hint: "List and view own files" },
  { id: "manage", label: "manage", hint: "Visibility, share, delete, tokens" },
  { id: "admin", label: "admin", hint: "Admin routes (role still required)" },
];

type ExpiryChoice = "never" | "30d" | "90d" | "1y";

function expiresAtISO(choice: ExpiryChoice): string | undefined {
  if (choice === "never") return undefined;
  const days = choice === "30d" ? 30 : choice === "90d" ? 90 : 365;
  return new Date(Date.now() + days * 24 * 60 * 60 * 1000).toISOString();
}

export function APITokensCard({ isAdmin }: { isAdmin: boolean }) {
  const [tokens, setTokens] = useState<APITokenMeta[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<APITokenScope[]>(["upload", "read"]);
  const [expiry, setExpiry] = useState<ExpiryChoice>("never");
  const [creating, setCreating] = useState(false);
  const [revokingId, setRevokingId] = useState<string | null>(null);
  const [revealed, setRevealed] = useState<APITokenCreated | null>(null);

  const reload = useCallback(async () => {
    const list = await listAPITokens();
    setTokens(list);
    setLoadError(null);
  }, []);

  useEffect(() => {
    let cancelled = false;
    void listAPITokens()
      .then((list) => {
        if (!cancelled) {
          setTokens(list);
          setLoadError(null);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setLoadError(
            err instanceof ApiError
              ? err.message
              : err instanceof Error
                ? err.message
                : "Failed to load tokens",
          );
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  function toggleScope(scope: APITokenScope, on: boolean) {
    setScopes((prev) => {
      if (on) return prev.includes(scope) ? prev : [...prev, scope];
      return prev.filter((s) => s !== scope);
    });
  }

  async function onCreate(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) {
      toast.error("Name is required");
      return;
    }
    if (scopes.length === 0) {
      toast.error("Pick at least one scope");
      return;
    }
    setCreating(true);
    try {
      const created = await createAPIToken({
        name: trimmed,
        scopes,
        expiresAt: expiresAtISO(expiry),
      });
      setRevealed(created);
      setName("");
      await reload();
      toast.success("Token created — copy it now");
    } catch (err) {
      toast.error(
        err instanceof ApiError
          ? err.message
          : err instanceof Error
            ? err.message
            : "Could not create token",
      );
    } finally {
      setCreating(false);
    }
  }

  async function onRevoke(id: string) {
    if (!window.confirm("Revoke this token? Scripts using it will fail immediately.")) {
      return;
    }
    setRevokingId(id);
    try {
      await revokeAPIToken(id);
      if (revealed?.id === id) setRevealed(null);
      await reload();
      toast.success("Token revoked");
    } catch (err) {
      toast.error(
        err instanceof ApiError
          ? err.message
          : err instanceof Error
            ? err.message
            : "Could not revoke token",
      );
    } finally {
      setRevokingId(null);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>API tokens</CardTitle>
        <CardDescription>
          Personal access tokens for automation (
          <code className="text-xs">dc_…</code>
          ). Max 20. Changing your password revokes all of them.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-6">
        {revealed ? (
          <Alert className="border-geist-amber/40 bg-geist-amber-soft">
            <AlertTitle>Copy this token now</AlertTitle>
            <AlertDescription className="flex flex-col gap-3">
              <p>
                Shown once for <span className="font-medium">{revealed.name}</span>.
                Closing dismisses it from this page.
              </p>
              <div className="flex flex-wrap items-center gap-2">
                <code className="max-w-full break-all rounded-md bg-background/80 px-2 py-1 text-xs">
                  {revealed.token}
                </code>
                <CopyButton value={revealed.token} label="Copy API token" />
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => setRevealed(null)}
                >
                  Dismiss
                </Button>
              </div>
            </AlertDescription>
          </Alert>
        ) : null}

        <form className="flex flex-col gap-3" onSubmit={onCreate}>
          <FieldGroup className="gap-3">
            <Field>
              <FieldLabel htmlFor="pat-name">Name</FieldLabel>
              <Input
                id="pat-name"
                value={name}
                maxLength={64}
                required
                placeholder="ci"
                disabled={creating}
                onChange={(e) => setName(e.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel>Scopes</FieldLabel>
              <div className="flex flex-col gap-2">
                {ALL_SCOPES.map((s) => {
                  if (s.id === "admin" && !isAdmin) return null;
                  const checked = scopes.includes(s.id);
                  return (
                    <label
                      key={s.id}
                      className="flex cursor-pointer items-start gap-2 text-sm"
                    >
                      <Checkbox
                        checked={checked}
                        disabled={creating}
                        onCheckedChange={(v) =>
                          toggleScope(s.id, v === true)
                        }
                        className="mt-0.5"
                      />
                      <span className="min-w-0">
                        <span className="font-medium">{s.label}</span>
                        <span className="block text-xs text-muted-foreground">
                          {s.hint}
                        </span>
                      </span>
                    </label>
                  );
                })}
              </div>
            </Field>
            <Field>
              <FieldLabel htmlFor="pat-expiry">Expires</FieldLabel>
              <Select
                value={expiry}
                disabled={creating}
                onValueChange={(v) => {
                  if (v === "never" || v === "30d" || v === "90d" || v === "1y") {
                    setExpiry(v);
                  }
                }}
              >
                <SelectTrigger id="pat-expiry" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="never">Never</SelectItem>
                    <SelectItem value="30d">30 days</SelectItem>
                    <SelectItem value="90d">90 days</SelectItem>
                    <SelectItem value="1y">1 year</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
          </FieldGroup>
          <Button type="submit" disabled={creating} className="self-start">
            {creating ? <Spinner data-icon="inline-start" /> : null}
            {creating ? "Creating…" : "Create token"}
          </Button>
        </form>

        <div className="flex flex-col gap-2">
          <h3 className="text-sm font-medium">Active tokens</h3>
          {loadError ? (
            <Alert variant="destructive">
              <AlertTitle>Could not load tokens</AlertTitle>
              <AlertDescription>{loadError}</AlertDescription>
            </Alert>
          ) : tokens === null ? (
            <Skeleton className="h-16 w-full" />
          ) : tokens.length === 0 ? (
            <p className="text-sm text-muted-foreground">No tokens yet.</p>
          ) : (
            <ul className="divide-y divide-border/60 rounded-lg border border-border/60">
              {tokens.map((t) => (
                <li
                  key={t.id}
                  className="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="min-w-0">
                    <p className="font-medium">{t.name}</p>
                    <div className="mt-1 flex flex-wrap gap-1">
                      {t.scopes.map((s) => (
                        <Badge key={s} variant="secondary">
                          {s}
                        </Badge>
                      ))}
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      Created {formatDate(t.createdAt)}
                      {t.expiresAt
                        ? ` · Expires ${formatDate(t.expiresAt)}`
                        : " · Never expires"}
                      {t.lastUsedAt
                        ? ` · Last used ${formatDate(t.lastUsedAt)}`
                        : ""}
                    </p>
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={revokingId === t.id}
                    onClick={() => void onRevoke(t.id)}
                  >
                    {revokingId === t.id ? (
                      <Spinner data-icon="inline-start" />
                    ) : null}
                    Revoke
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
