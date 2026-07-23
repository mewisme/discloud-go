"use client";

import Script from "next/script";
import { useEffect, useRef, useState } from "react";

import {
  fetchCaptchaConfig,
  notifyCaptchaReset,
  setCaptchaToken,
} from "@/lib/captcha";

declare global {
  interface Window {
    turnstile?: {
      render: (
        el: HTMLElement,
        opts: {
          sitekey: string;
          action?: string;
          callback?: (token: string) => void;
          "expired-callback"?: () => void;
          "error-callback"?: () => void;
        },
      ) => string;
      reset: (widgetId?: string) => void;
      remove: (widgetId?: string) => void;
    };
  }
}

type Props = {
  /** When false, unmount widget (e.g. signed-in). */
  active?: boolean;
};

/**
 * Cloudflare Turnstile widget. Fetches site key from `/api/captcha` (CAPTCHA_KEY).
 * Tokens are single-use; upload code calls takeCaptchaToken() which resets after consume.
 */
export function TurnstileWidget({ active = true }: Props) {
  const [sitekey, setSitekey] = useState<string | null>(null);
  const hostRef = useRef<HTMLDivElement>(null);
  const widgetIdRef = useRef<string | null>(null);

  useEffect(() => {
    if (!active) return;
    let cancelled = false;
    void fetchCaptchaConfig().then((cfg) => {
      if (!cancelled) setSitekey(cfg.enabled ? cfg.siteKey : "");
    });
    return () => {
      cancelled = true;
    };
  }, [active]);

  const key = active ? sitekey : "";

  useEffect(() => {
    if (!key) return;

    let cancelled = false;

    const mount = () => {
      if (cancelled || !hostRef.current || !window.turnstile) return;
      if (widgetIdRef.current) {
        try {
          window.turnstile.remove(widgetIdRef.current);
        } catch {
          /* ignore */
        }
        widgetIdRef.current = null;
      }
      hostRef.current.innerHTML = "";
      widgetIdRef.current = window.turnstile.render(hostRef.current, {
        sitekey: key,
        action: "turnstile-spin-v2",
        callback: (t) => setCaptchaToken(t),
        "expired-callback": () => setCaptchaToken(""),
        "error-callback": () => setCaptchaToken(""),
      });
      notifyCaptchaReset(() => {
        if (widgetIdRef.current && window.turnstile) {
          window.turnstile.reset(widgetIdRef.current);
        }
      });
    };

    if (window.turnstile) {
      mount();
    } else {
      const onReady = () => mount();
      window.addEventListener("turnstile-script-ready", onReady);
      return () => {
        cancelled = true;
        window.removeEventListener("turnstile-script-ready", onReady);
        notifyCaptchaReset(null);
        if (widgetIdRef.current && window.turnstile) {
          try {
            window.turnstile.remove(widgetIdRef.current);
          } catch {
            /* ignore */
          }
        }
      };
    }

    return () => {
      cancelled = true;
      notifyCaptchaReset(null);
      if (widgetIdRef.current && window.turnstile) {
        try {
          window.turnstile.remove(widgetIdRef.current);
        } catch {
          /* ignore */
        }
        widgetIdRef.current = null;
      }
    };
  }, [key]);

  if (!active || !key) return null;

  return (
    <>
      <Script
        src="https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit"
        strategy="afterInteractive"
        onLoad={() => {
          window.dispatchEvent(new Event("turnstile-script-ready"));
        }}
      />
      <div
        ref={hostRef}
        className="cf-turnstile flex justify-center"
        data-action="turnstile-spin-v2"
      />
    </>
  );
}
