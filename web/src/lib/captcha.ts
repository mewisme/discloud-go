/** Client-side Turnstile token handoff for anon upload session create. */

export type CaptchaConfig = {
  enabled: boolean;
  siteKey: string;
  provider: "turnstile" | null;
};

let cachedConfig: CaptchaConfig | null = null;
let configPromise: Promise<CaptchaConfig> | null = null;

let token = "";
let resetFn: (() => void) | null = null;
const waiters: Array<(t: string) => void> = [];

export async function fetchCaptchaConfig(): Promise<CaptchaConfig> {
  if (cachedConfig) return cachedConfig;
  if (!configPromise) {
    configPromise = fetch("/api/captcha", { cache: "no-store" })
      .then(async (res) => {
        if (!res.ok) {
          return { enabled: false, siteKey: "", provider: null } satisfies CaptchaConfig;
        }
        const body = (await res.json()) as Partial<CaptchaConfig>;
        const siteKey = typeof body.siteKey === "string" ? body.siteKey.trim() : "";
        const cfg: CaptchaConfig = {
          enabled: Boolean(body.enabled && siteKey),
          siteKey,
          provider: siteKey ? "turnstile" : null,
        };
        cachedConfig = cfg;
        return cfg;
      })
      .catch(() => {
        const cfg: CaptchaConfig = { enabled: false, siteKey: "", provider: null };
        cachedConfig = cfg;
        return cfg;
      });
  }
  return configPromise;
}

export function setCaptchaToken(next: string): void {
  token = next.trim();
  if (!token) return;
  while (waiters.length) {
    waiters.shift()?.(token);
  }
}

export function notifyCaptchaReset(fn: (() => void) | null): void {
  resetFn = fn;
}

function resetWidget(): void {
  token = "";
  resetFn?.();
}

/**
 * Returns a fresh Turnstile token when the widget is mounted; otherwise undefined.
 * Consumes the token and resets the widget so the next upload gets a new one.
 */
export function takeCaptchaToken(signal?: AbortSignal): Promise<string | undefined> {
  // No widget mounted (signed-in, or captcha disabled) → do not block.
  if (!resetFn && !token) return Promise.resolve(undefined);

  const take = (t: string) => {
    token = "";
    queueMicrotask(() => resetWidget());
    return t;
  };

  if (token) return Promise.resolve(take(token));

  return new Promise((resolve, reject) => {
    const onAbort = () => {
      const i = waiters.indexOf(waiter);
      if (i >= 0) waiters.splice(i, 1);
      reject(signal?.reason ?? new DOMException("Upload cancelled", "AbortError"));
    };
    const waiter = (t: string) => {
      signal?.removeEventListener("abort", onAbort);
      resolve(take(t));
    };
    if (signal?.aborted) {
      onAbort();
      return;
    }
    signal?.addEventListener("abort", onAbort, { once: true });
    waiters.push(waiter);
  });
}
