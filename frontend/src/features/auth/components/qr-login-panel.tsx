"use client";

import { Centrifuge, type Subscription } from "centrifuge";
import { Loader2, RefreshCw, Smartphone } from "lucide-react";
import { signIn } from "next-auth/react";
import QRCode from "qrcode";
import { useCallback, useEffect, useRef, useState } from "react";

import { brand } from "@/config/brand";
import { Button } from "@/components/ui/button";
import { realtimeConfig } from "@/config/realtime";
import {
  resolveCredentialErrorCode,
  type CredentialSignInResult,
} from "@/lib/auth/credentials-errors";
import { useLocale } from "@/providers/locale-provider";
import { authService, type QRLoginCreated } from "@/services/auth.service";

/** Auto-refreshes before the user has to click (≈ 10 minutes of QRs). */
const MAX_AUTO_REFRESHES = 5;
/** Ask for a fresh QR this long before the current one expires. */
const REFRESH_LEAD_MS = 3_000;

type Phase =
  | "loading"
  | "ready"
  | "scanned"
  | "signing_in"
  | "rejected"
  | "idle"
  | "error";

type QREvent = { type?: string; exchange_token?: string };

/**
 * QR sign-in for the web login page. The tab gets a short-lived session plus
 * an anonymous Centrifugo token for its own private channel and waits for the
 * mobile app to approve — no polling. This is the only page that opens its
 * own Centrifugo client: the app-wide ConnectionManager needs a signed-in user.
 */
export function QRLoginPanel({
  onSignedIn,
  onErrorCode,
}: {
  /** Finishes the login (session refresh + redirect). */
  onSignedIn: () => Promise<boolean>;
  /** Maps a credentials error code to a message on the parent form. */
  onErrorCode: (code: string | null) => void;
}) {
  const { t } = useLocale();
  const [phase, setPhase] = useState<Phase>("loading");
  const [qrDataUrl, setQrDataUrl] = useState<string | null>(null);
  const sessionRef = useRef<QRLoginCreated | null>(null);
  const clientRef = useRef<Centrifuge | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const refreshesRef = useRef(0);
  const exchangingRef = useRef(false);
  const mountedRef = useRef(true);
  const startRef = useRef<(manual: boolean) => void>(() => undefined);
  // Parent callbacks change every render; keep them out of the effect deps
  // so a re-render never restarts the QR session.
  const onSignedInRef = useRef(onSignedIn);
  const onErrorCodeRef = useRef(onErrorCode);
  useEffect(() => {
    onSignedInRef.current = onSignedIn;
    onErrorCodeRef.current = onErrorCode;
  }, [onSignedIn, onErrorCode]);

  const teardown = useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = null;
    clientRef.current?.disconnect();
    clientRef.current = null;
  }, []);

  const exchange = useCallback(
    async (session: QRLoginCreated, exchangeToken: string) => {
      if (exchangingRef.current || sessionRef.current !== session) return;
      exchangingRef.current = true;
      teardown();
      setPhase("signing_in");
      try {
        const result = (await signIn("qr-login", {
          session_id: session.session_id,
          browser_secret: session.browser_secret,
          exchange_token: exchangeToken,
          redirect: false,
        })) as CredentialSignInResult | undefined;
        const code = result ? resolveCredentialErrorCode(result) : null;
        if (code || !result?.ok) {
          onErrorCodeRef.current(code);
          if (mountedRef.current) setPhase("error");
          return;
        }
        const ok = await onSignedInRef.current();
        if (!ok && mountedRef.current) setPhase("error");
      } catch {
        onErrorCodeRef.current(null);
        if (mountedRef.current) setPhase("error");
      } finally {
        exchangingRef.current = false;
      }
    },
    [teardown],
  );

  const handleEvent = useCallback(
    (session: QRLoginCreated, event: QREvent) => {
      if (sessionRef.current !== session) return;
      switch (event.type) {
        case "scanned":
          setPhase((p) => (p === "ready" ? "scanned" : p));
          break;
        case "approved":
          if (event.exchange_token)
            void exchange(session, event.exchange_token);
          break;
        case "rejected":
          teardown();
          setPhase("rejected");
          break;
      }
    },
    [exchange, teardown],
  );

  const start = useCallback(
    async (manual: boolean) => {
      teardown();
      if (manual) refreshesRef.current = 0;
      setPhase("loading");
      let session: QRLoginCreated;
      try {
        session = await authService.createQRLoginSession();
      } catch {
        if (mountedRef.current) setPhase("error");
        return;
      }
      if (!mountedRef.current) return;
      sessionRef.current = session;
      const dataUrl = await QRCode.toDataURL(session.qr_url, {
        width: 224,
        margin: 1,
        errorCorrectionLevel: "M",
      });
      if (!mountedRef.current || sessionRef.current !== session) return;
      setQrDataUrl(dataUrl);
      setPhase("ready");

      const client = new Centrifuge(
        session.realtime.ws_url || realtimeConfig.wsUrl,
        { token: session.realtime.connection_token },
      );
      const sub: Subscription = client.newSubscription(
        session.realtime.channel,
        { token: session.realtime.subscription_token },
      );
      sub.on("publication", (ctx) => handleEvent(session, ctx.data as QREvent));
      // Catch up once per (re)subscribe in case an event was sent while the
      // socket was down. Not a poll: it only runs on subscribe.
      sub.on("subscribed", () => {
        void authService
          .getQRLoginState(session.session_id, session.browser_secret)
          .then((state) => {
            if (state.status === "approved" && state.exchange_token) {
              handleEvent(session, {
                type: "approved",
                exchange_token: state.exchange_token,
              });
            } else if (
              state.status === "scanned" ||
              state.status === "rejected"
            ) {
              handleEvent(session, { type: state.status });
            }
          })
          .catch(() => undefined);
      });
      sub.subscribe();
      client.connect();
      clientRef.current = client;

      const msLeft =
        new Date(session.expires_at).getTime() - Date.now() - REFRESH_LEAD_MS;
      timerRef.current = setTimeout(
        () => {
          if (exchangingRef.current) return;
          const hidden =
            typeof document !== "undefined" &&
            document.visibilityState === "hidden";
          if (hidden || refreshesRef.current >= MAX_AUTO_REFRESHES) {
            teardown();
            setPhase("idle");
            return;
          }
          refreshesRef.current += 1;
          startRef.current(false);
        },
        Math.max(msLeft, 5_000),
      );
    },
    [handleEvent, teardown],
  );

  useEffect(() => {
    startRef.current = (manual) => void start(manual);
  }, [start]);

  useEffect(() => {
    mountedRef.current = true;
    // Deferred so no state update runs synchronously inside the effect.
    const first = setTimeout(() => void start(true), 0);
    return () => {
      clearTimeout(first);
      mountedRef.current = false;
      sessionRef.current = null;
      teardown();
    };
  }, [start, teardown]);

  // A tab that went idle while hidden gets a fresh QR when it is shown again.
  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState === "visible" && phase === "idle") {
        startRef.current(true);
      }
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, [phase]);

  const showQR = phase === "ready" || phase === "scanned";

  return (
    <div className="flex flex-col items-center gap-4">
      <div className="bg-background relative flex h-[240px] w-[240px] items-center justify-center rounded-md border p-2">
        {showQR && qrDataUrl ? (
          // QR is a data URL from qrcode; next/image is not applicable.
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={qrDataUrl}
            alt={t("auth.qr.alt")}
            width={224}
            height={224}
            className={phase === "scanned" ? "opacity-20" : undefined}
          />
        ) : null}
        {phase === "loading" || phase === "signing_in" ? (
          <Loader2 className="text-muted-foreground size-6 animate-spin" />
        ) : null}
        {phase === "scanned" ? (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 p-4 text-center">
            <Smartphone className="size-8" aria-hidden />
            <p className="text-sm font-medium">{t("auth.qr.scanned")}</p>
          </div>
        ) : null}
        {phase === "rejected" || phase === "idle" || phase === "error" ? (
          <div className="flex flex-col items-center gap-3 p-4 text-center">
            <p className="text-muted-foreground text-sm">
              {phase === "rejected"
                ? t("auth.qr.rejected")
                : phase === "idle"
                  ? t("auth.qr.expired")
                  : t("auth.qr.error")}
            </p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => startRef.current(true)}
            >
              <RefreshCw aria-hidden />
              {t("auth.qr.refresh")}
            </Button>
          </div>
        ) : null}
      </div>
      <ol className="text-muted-foreground list-decimal space-y-1 pl-5 text-sm">
        <li>{t("auth.qr.step_open", { app: brand.name })}</li>
        <li>{t("auth.qr.step_scan")}</li>
        <li>{t("auth.qr.step_approve")}</li>
      </ol>
      <p className="text-muted-foreground text-center text-xs">
        {phase === "signing_in"
          ? t("auth.qr.signing_in")
          : t("auth.qr.auto_refresh")}
      </p>
    </div>
  );
}
