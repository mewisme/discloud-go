"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState, useSyncExternalStore } from "react";
import { toast } from "sonner";

import { userInitial } from "@/components/auth-header";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
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
import { Progress } from "@/components/ui/progress";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import {
  ApiError,
  changePassword,
  fetchAccountMe,
  updatePreferences,
  type AccountMe,
} from "@/lib/api";
import {
  ensureAuth,
  getAuthServerSnapshot,
  getAuthSnapshot,
  signOutAndClear,
  subscribeAuth,
} from "@/lib/auth";
import { formatBytes, formatDate } from "@/lib/format";
import { parseUserAgent } from "@/lib/user-agent";

function StatCard({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint?: string;
}) {
  return (
    <Card size="sm" className="gap-2">
      <CardHeader className="pb-0">
        <CardDescription>{label}</CardDescription>
        <CardTitle className="text-2xl tabular-nums tracking-tight">
          {value}
        </CardTitle>
      </CardHeader>
      {hint ? (
        <CardContent className="pt-0 text-xs text-muted-foreground">
          {hint}
        </CardContent>
      ) : null}
    </Card>
  );
}

function MetaRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 py-2 text-sm">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      <span className="min-w-0 text-right break-all">{children}</span>
    </div>
  );
}

export function MePanel() {
  const router = useRouter();
  const user = useSyncExternalStore(
    subscribeAuth,
    getAuthSnapshot,
    getAuthServerSnapshot,
  );
  const [account, setAccount] = useState<AccountMe | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [securityOpen, setSecurityOpen] = useState<string[]>([]);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [savingVisibility, setSavingVisibility] = useState(false);

  useEffect(() => {
    void ensureAuth();
  }, []);

  useEffect(() => {
    if (user === null) {
      router.replace("/");
    }
  }, [user, router]);

  useEffect(() => {
    if (!user) return;
    let cancelled = false;
    void fetchAccountMe()
      .then((me) => {
        if (!cancelled) {
          setAccount(me);
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
                : "Failed to load account",
          );
        }
      });
    return () => {
      cancelled = true;
    };
  }, [user]);

  if (user === undefined || user === null || (!account && !loadError)) {
    return (
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
        <div className="flex items-center gap-4">
          <Skeleton className="size-12 rounded-full" />
          <div className="flex flex-col gap-2">
            <Skeleton className="h-6 w-40" />
            <Skeleton className="h-4 w-24" />
          </div>
        </div>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-24 w-full" />
          ))}
        </div>
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  if (loadError || !account) {
    return (
      <Alert variant="destructive" className="mx-auto w-full max-w-3xl">
        <AlertTitle>Could not load account</AlertTitle>
        <AlertDescription>{loadError ?? "Failed to load account"}</AlertDescription>
      </Alert>
    );
  }

  const { browser, os } = parseUserAgent(account.session.userAgent);
  const expiringPct =
    account.stats.fileCount > 0
      ? Math.round(
        (100 * account.stats.expiringSoonCount) / account.stats.fileCount,
      )
      : 0;

  async function onChangePassword(e: React.FormEvent) {
    e.preventDefault();
    if (newPassword.length < 8) {
      toast.error("Password must be at least 8 characters");
      return;
    }
    if (newPassword !== confirmPassword) {
      toast.error("New passwords do not match");
      return;
    }
    setBusy(true);
    try {
      await changePassword(currentPassword, newPassword);
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      setSecurityOpen([]);
      const next = await fetchAccountMe();
      setAccount(next);
      toast.success("Password updated");
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
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <div className="flex items-center gap-4">
        <Avatar size="lg">
          <AvatarFallback className="text-base">
            {userInitial(account.username)}
          </AvatarFallback>
        </Avatar>
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="truncate text-xl font-semibold tracking-tight">
              {account.username}
            </h1>
            <Badge variant={account.role === "admin" ? "default" : "secondary"}>
              {account.role}
            </Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            Member since {formatDate(account.createdAt)}
          </p>
        </div>
      </div>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-medium">Overview</h2>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard label="Uploads" value={String(account.stats.fileCount)} />
          <StatCard
            label="Storage"
            value={formatBytes(account.stats.totalBytes)}
          />
          <StatCard
            label="Visibility"
            value={`${account.stats.publicCount} / ${account.stats.privateCount}`}
            hint="Public / private"
          />
          <StatCard
            label="Expiring soon"
            value={String(account.stats.expiringSoonCount)}
            hint="Within 7 days"
          />
        </div>
        {account.stats.fileCount > 0 ? (
          <div className="flex flex-col gap-1.5">
            <div className="flex w-full items-center justify-between gap-3 text-xs text-muted-foreground">
              <span>Files expiring within 7 days</span>
              <span className="shrink-0 tabular-nums">
                {account.stats.expiringSoonCount}/{account.stats.fileCount}
              </span>
            </div>
            <Progress value={expiringPct} />
          </div>
        ) : null}
      </section>

      <Card>
        <CardHeader>
          <CardTitle>Retention</CardTitle>
          <CardDescription>
            How long files stay available before cleanup.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-2 text-sm text-muted-foreground">
          <p>
            Signed-in uploads expire after{" "}
            <span className="font-medium text-foreground">
              {account.retention.authenticatedDays} days
            </span>
            .
          </p>
          <p>
            Anonymous uploads expire after{" "}
            <span className="font-medium text-foreground">
              {account.retention.anonymousDays} days
            </span>
            .
          </p>
          <p>
            A full download extends expiry by{" "}
            <span className="font-medium text-foreground">
              {account.retention.downloadExtensionDays} days
            </span>
            , capped at{" "}
            <span className="font-medium text-foreground">
              {account.retention.maxRetentionDays} days
            </span>{" "}
            from now.
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Security</CardTitle>
          <CardDescription>
            Password last changed {formatDate(account.passwordChangedAt)}.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Accordion
            value={securityOpen}
            onValueChange={setSecurityOpen}
            className="border-t border-border/60"
          >
            <AccordionItem value="password" className="border-b-0">
              <AccordionTrigger className="hover:no-underline">
                Change password
              </AccordionTrigger>
              <AccordionContent className="h-auto pb-1">
                {securityOpen.includes("password") ? (
                  <form
                    className="flex flex-col gap-3 pt-1"
                    onSubmit={onChangePassword}
                  >
                    <p className="text-xs text-muted-foreground">
                      Your current session stays signed in after a password
                      change.
                    </p>
                    <FieldGroup className="gap-3">
                      <Field>
                        <FieldLabel htmlFor="current-password">
                          Current password
                        </FieldLabel>
                        <Input
                          id="current-password"
                          type="password"
                          autoComplete="current-password"
                          required
                          value={currentPassword}
                          onChange={(e) => setCurrentPassword(e.target.value)}
                          disabled={busy}
                        />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor="new-password">
                          New password
                        </FieldLabel>
                        <Input
                          id="new-password"
                          type="password"
                          autoComplete="new-password"
                          required
                          minLength={8}
                          value={newPassword}
                          onChange={(e) => setNewPassword(e.target.value)}
                          disabled={busy}
                        />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor="confirm-password">
                          Confirm new password
                        </FieldLabel>
                        <Input
                          id="confirm-password"
                          type="password"
                          autoComplete="new-password"
                          required
                          minLength={8}
                          value={confirmPassword}
                          onChange={(e) => setConfirmPassword(e.target.value)}
                          disabled={busy}
                        />
                      </Field>
                    </FieldGroup>
                    <Button type="submit" disabled={busy} className="self-start">
                      {busy ? <Spinner data-icon="inline-start" /> : null}
                      {busy ? "Updating…" : "Update password"}
                    </Button>
                  </form>
                ) : null}
              </AccordionContent>
            </AccordionItem>
          </Accordion>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Current session</CardTitle>
          <CardDescription>This browser only — not a session list.</CardDescription>
        </CardHeader>
        <CardContent className="divide-y divide-border/60">
          <MetaRow label="Browser">
            <Badge variant="secondary">{browser}</Badge>
          </MetaRow>
          <MetaRow label="OS">
            <Badge variant="secondary">{os}</Badge>
          </MetaRow>
          <MetaRow label="IP">{account.session.ip || "—"}</MetaRow>
          <MetaRow label="Signed in">
            {formatDate(account.session.createdAt)}
          </MetaRow>
          <MetaRow label="Last activity">
            {formatDate(account.session.lastSeenAt)}
          </MetaRow>
          <MetaRow label="Expires">
            {formatDate(account.session.expiresAt)}
          </MetaRow>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Upload preferences</CardTitle>
          <CardDescription>
            Applied to new uploads from this account.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <Field>
            <FieldLabel htmlFor="default-visibility">
              Default visibility
            </FieldLabel>
            <Select
              value={account.preferences?.defaultVisibility ?? "public"}
              disabled={savingVisibility}
              onValueChange={(v) => {
                if (v !== "public" && v !== "private") return;
                const next = v;
                const prev = account.preferences?.defaultVisibility ?? "public";
                if (next === prev) return;
                setSavingVisibility(true);
                void updatePreferences({ defaultVisibility: next })
                  .then((res) => {
                    setAccount({
                      ...account,
                      preferences: res.preferences,
                    });
                    toast.success(
                      next === "private"
                        ? "New uploads will be private"
                        : "New uploads will be public",
                    );
                  })
                  .catch((err) => {
                    toast.error(
                      err instanceof ApiError
                        ? err.message
                        : "Could not save preference",
                    );
                  })
                  .finally(() => setSavingVisibility(false));
              }}
            >
              <SelectTrigger
                id="default-visibility"
                className="w-full"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="public">Public</SelectItem>
                  <SelectItem value="private">Private</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <Field orientation="horizontal" data-disabled className="opacity-60">
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <FieldLabel htmlFor="prefer-chunked">
                Prefer chunked upload for large files
              </FieldLabel>
              <Badge variant="outline" className="w-fit">
                Coming soon
              </Badge>
            </div>
            <Switch id="prefer-chunked" disabled defaultChecked />
          </Field>
        </CardContent>
      </Card>

      <Card className="border-destructive/30">
        <CardHeader>
          <CardTitle className="text-destructive">Danger zone</CardTitle>
          <CardDescription>
            Sign out ends this session on this device.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Separator className="mb-4" />
          <Button
            type="button"
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
            Sign out
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
