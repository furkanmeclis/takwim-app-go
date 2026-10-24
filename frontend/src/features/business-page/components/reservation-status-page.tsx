"use client";

import {
  CalendarCheck,
  CircleCheck,
  CircleX,
  Clock,
  CreditCard,
  ExternalLink,
  Loader2,
} from "lucide-react";
import Link from "next/link";
import { useState, useSyncExternalStore } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { useBookingSurface } from "@/features/business-page/components/booking-surface";
import {
  BusinessPageShell,
  CustomerPanelLink,
} from "@/features/business-page/components/business-page-shell";
import {
  usePublicBusiness,
  usePublicReservation,
  useStartPublicPayment,
} from "@/features/business-page/hooks/use-public-booking";
import { readAccessToken } from "@/features/business-page/lib/access";
import { reportPaymentError } from "@/features/business-page/lib/errors";
import {
  flowHasParty,
  seatingLabel,
  vehicleText,
} from "@/features/business-page/lib/wizard";
import type {
  PublicBusiness,
  PublicReservation,
} from "@/features/business-page/types";
import {
  ApprovalChip,
  ConfirmationChip,
  PaymentChip,
  ReservationStatusChip,
} from "@/features/reservations/components/reservation-status-chips";
import { priceLabel } from "@/features/services/lib/format";
import { isApiError } from "@/lib/api";
import { datetimeInZone } from "@/lib/utils/format";
import { answerText } from "@/features/reservations/lib/custom-fields";
import { useLocale } from "@/providers/locale-provider";

const noop = () => () => {};

/** How long the embedded screen watches a payment opened in a new tab. */
const PAYMENT_TAB_WATCH_MS = 15 * 60_000;

type Headline = {
  icon: typeof CircleCheck;
  tone: string;
  title: string;
  description: string;
};

/** What the customer should know first about the booking's state. */
function headline(
  r: PublicReservation,
  t: (key: string, params?: Record<string, string | number>) => string,
  until: string,
): Headline {
  if (r.status === "cancelled" || r.status === "rejected") {
    return {
      icon: CircleX,
      tone: "text-destructive",
      title: t(`business_page.status.${r.status}_title`),
      description: t(`business_page.status.${r.status}_description`),
    };
  }
  if (r.status === "requested" && r.approval_status === "pending") {
    return {
      icon: Clock,
      tone: "text-amber-600 dark:text-amber-400",
      title: t("business_page.status.pending_title"),
      description: until
        ? t("business_page.status.pending_until", { until })
        : t("business_page.status.pending_description"),
    };
  }
  if (r.payment_status === "online_pending") {
    return {
      icon: CreditCard,
      tone: "text-amber-600 dark:text-amber-400",
      title: t("business_page.status.payment_title"),
      description: until
        ? t("business_page.status.payment_until", { until })
        : t("business_page.status.payment_description"),
    };
  }
  if (r.status === "completed" || r.status === "no_show") {
    return {
      icon: CalendarCheck,
      tone: "text-muted-foreground",
      title: t(`business_page.status.${r.status}_title`),
      description: "",
    };
  }
  return {
    icon: CircleCheck,
    tone: "text-emerald-600 dark:text-emerald-400",
    title: t("business_page.status.confirmed_title"),
    description: t("business_page.status.confirmed_description"),
  };
}

type PaymentReturn = "done" | "failed" | null;

/** `?payment=done|failed` of the provider's return link (proves nothing). */
function readPaymentReturn(): PaymentReturn {
  const value = new URLSearchParams(window.location.search).get("payment");
  return value === "done" || value === "failed" ? value : null;
}

/** Only http(s) pages of the provider are opened. */
function safePaymentURL(raw: string) {
  try {
    const url = new URL(raw, window.location.origin);
    return url.protocol === "https:" || url.protocol === "http:"
      ? url.toString()
      : null;
  } catch {
    return null;
  }
}

/**
 * Online payment of the booking: the "pay now" button while the server says
 * it can be paid, and what the return from the provider's page means. The
 * state shown is always the server's `payment_status`, recorded only from
 * the provider's verified notification.
 *
 * Inside the embedded screen (TEC-908) the provider's page never opens in
 * the frame: the customer's click opens a new tab, and this frame watches
 * the server's state until the payment is recorded.
 */
function PaymentPanel({
  reservation: r,
  slug,
  token,
  returned,
  onOpened,
}: {
  reservation: PublicReservation;
  slug: string;
  token: string;
  returned: PaymentReturn;
  onOpened: () => void;
}) {
  const { t, locale } = useLocale();
  const { embed } = useBookingSurface();
  const start = useStartPublicPayment(slug, r.uuid, token);
  const [openedURL, setOpenedURL] = useState<string | null>(null);
  const pending = r.payment_status === "online_pending";

  let note: { title?: string; text: string; destructive?: boolean } | null =
    null;
  if (returned === "done" && pending) {
    note = {
      title: t("business_page.status.payment_checking_title"),
      text: t("business_page.status.payment_checking"),
    };
  } else if (returned === "failed" && pending) {
    note = {
      title: t("business_page.status.payment_failed_title"),
      text: t("business_page.status.payment_failed"),
      destructive: true,
    };
  } else if (returned && r.payment_status === "paid") {
    note = { text: t("business_page.status.payment_done") };
  } else if (returned && r.payment_status === "refund_pending") {
    note = { text: t("business_page.status.payment_late"), destructive: true };
  }

  if (openedURL && pending && !note) {
    note = {
      title: t("business_page.status.payment_tab_title"),
      text: t("business_page.status.payment_tab"),
    };
  }

  if (!note && !r.can_pay_online) return null;

  const pay = () => {
    if (!embed) {
      start.mutate(locale, {
        onSuccess: (page) => window.location.assign(page.redirect_url),
        onError: (error) => reportPaymentError(error, t),
      });
      return;
    }
    // Opened during the click so no popup blocker stops it; it gets the
    // provider's address once the API answers.
    const tab = window.open("", "_blank");
    if (tab) tab.opener = null;
    start.mutate(locale, {
      onSuccess: (page) => {
        const url = safePaymentURL(page.redirect_url);
        if (!url) {
          tab?.close();
          return;
        }
        if (tab) tab.location.href = url;
        setOpenedURL(url);
        onOpened();
      },
      onError: (error) => {
        tab?.close();
        reportPaymentError(error, t);
      },
    });
  };
  // The browser leaves for the provider's page right after success.
  const busy = start.isPending || (!embed && start.isSuccess);

  return (
    <div className="space-y-3">
      {note ? (
        <Alert variant={note.destructive ? "destructive" : "default"}>
          {note.destructive ? <CircleX /> : <Clock />}
          {note.title ? <AlertTitle>{note.title}</AlertTitle> : null}
          <AlertDescription>{note.text}</AlertDescription>
        </Alert>
      ) : null}
      {r.can_pay_online ? (
        <Card className="gap-3 py-4">
          <CardContent className="space-y-3 px-4">
            {openedURL ? (
              <Button asChild className="w-full" variant="outline">
                <a href={openedURL} target="_blank" rel="noopener noreferrer">
                  <ExternalLink />
                  {t("business_page.status.pay_open_tab")}
                </a>
              </Button>
            ) : (
              <Button className="w-full" onClick={pay} disabled={busy}>
                {busy ? <Loader2 className="animate-spin" /> : <CreditCard />}
                {busy
                  ? t("business_page.status.pay_redirecting")
                  : t("business_page.status.pay_now")}
              </Button>
            )}
            <p className="text-muted-foreground text-center text-xs">
              {t("business_page.status.pay_secure")}
            </p>
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}

function ReservationView({
  reservation: r,
  business,
  token,
  returned,
  onPaymentOpened,
}: {
  reservation: PublicReservation;
  business: PublicBusiness;
  token: string;
  returned: PaymentReturn;
  onPaymentOpened: () => void;
}) {
  const { t, locale } = useLocale();
  const surface = useBookingSurface();
  const zone = r.timezone || business.timezone;
  const time = (iso: string) =>
    datetimeInZone(iso, zone, locale, { hour: "2-digit", minute: "2-digit" });
  const until = r.expires_at
    ? datetimeInZone(r.expires_at, zone, locale, {
        day: "numeric",
        month: "long",
        hour: "2-digit",
        minute: "2-digit",
      })
    : "";
  const head = headline(r, t, until);
  const Icon = head.icon;
  const money = (amount: string) =>
    priceLabel(amount, r.currency || business.currency, locale);

  return (
    <div className="space-y-4">
      <Card className="items-center gap-2 py-6 text-center">
        <Icon className={`size-10 ${head.tone}`} />
        <CardHeader className="w-full px-4">
          <CardTitle className="text-lg">{head.title}</CardTitle>
          {head.description ? (
            <CardDescription>{head.description}</CardDescription>
          ) : null}
        </CardHeader>
      </Card>

      <PaymentPanel
        reservation={r}
        slug={business.slug}
        token={token}
        returned={returned}
        onOpened={onPaymentOpened}
      />

      <Card className="gap-3 py-4">
        <CardHeader className="px-4">
          <CardTitle>
            {datetimeInZone(r.starts_at, zone, locale, {
              weekday: "long",
              day: "numeric",
              month: "long",
              year: "numeric",
              hour: "2-digit",
              minute: "2-digit",
            })}
          </CardTitle>
          <CardDescription>
            {[
              r.branch.name,
              seatingLabel(
                t,
                r.party_size,
                r.zone,
                flowHasParty(business.flow),
              ),
              vehicleText(t, r.vehicle),
              t("business_page.status.number", { number: r.number }),
            ]
              .filter(Boolean)
              .join(" · ")}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3 px-4">
          <div className="flex flex-wrap gap-2">
            <ReservationStatusChip status={r.status} />
            <ApprovalChip status={r.approval_status} />
            <PaymentChip status={r.payment_status} />
            {r.confirmation_status !== "not_required" ? (
              <ConfirmationChip status={r.confirmation_status} />
            ) : null}
          </div>
          <ul className="space-y-2 text-sm">
            {r.lines.map((line, index) => (
              <li
                key={`${line.service_uuid ?? "line"}-${index}`}
                className="flex justify-between gap-3"
              >
                <span>
                  <span className="block font-medium">{line.service_name}</span>
                  <span className="text-muted-foreground block tabular-nums">
                    {time(line.starts_at)}–{time(line.ends_at)}
                    {line.staff ? ` · ${line.staff.name}` : ""}
                  </span>
                  {line.package_name ? (
                    <span className="text-muted-foreground block text-xs">
                      {t("business_page.packages.paid_with", {
                        name: line.package_name,
                      })}
                    </span>
                  ) : null}
                  {line.bundle_name ? (
                    <span className="text-muted-foreground block text-xs">
                      {t("business_page.bundle.in_bundle", {
                        name: line.bundle_name,
                      })}
                    </span>
                  ) : null}
                </span>
                <span className="shrink-0 text-end tabular-nums">
                  {line.package_name ? (
                    t("business_page.packages.session")
                  ) : (
                    <>
                      {line.list_price &&
                      line.list_price !== line.unit_price ? (
                        <span className="text-muted-foreground me-2 text-xs line-through">
                          {money(line.list_price)}
                        </span>
                      ) : null}
                      {money(line.unit_price)}
                    </>
                  )}
                </span>
              </li>
            ))}
            {r.products
              .filter((p) => p.status !== "released")
              .map((p, index) => (
                <li
                  key={`product-${index}`}
                  className="flex justify-between gap-3"
                >
                  <span>
                    <span className="block font-medium">
                      {p.variant_name
                        ? `${p.name} / ${p.variant_name}`
                        : p.name}
                    </span>
                    <span className="text-muted-foreground block tabular-nums">
                      {t("business_page.products.line", {
                        quantity: p.quantity,
                      })}
                    </span>
                  </span>
                  <span className="shrink-0 tabular-nums">
                    {money((Number(p.unit_price) * p.quantity).toFixed(2))}
                  </span>
                </li>
              ))}
          </ul>
          <Separator />
          <p className="flex justify-between text-sm font-semibold">
            <span>{t("business_page.summary.total")}</span>
            <span className="tabular-nums">{money(r.total_price)}</span>
          </p>
          {r.customer_note ? (
            <p className="text-muted-foreground text-sm whitespace-pre-line">
              {r.customer_note}
            </p>
          ) : null}
          {r.answers.map((answer) => (
            <p key={answer.key} className="text-muted-foreground text-sm">
              <span className="text-foreground font-medium">
                {answer.label}:
              </span>{" "}
              {answerText(answer, t)}
            </p>
          ))}
        </CardContent>
      </Card>

      {business.policies.cancellation.trim() ? (
        <p className="text-muted-foreground text-xs whitespace-pre-line">
          <span className="font-medium">
            {t("business_page.policies.cancellation")}:
          </span>{" "}
          {business.policies.cancellation}
        </p>
      ) : null}

      <div className="flex flex-col gap-2 sm:flex-row">
        <Button asChild className="sm:flex-1">
          <CustomerPanelLink>
            {t("business_page.my_bookings")}
          </CustomerPanelLink>
        </Button>
        <Button asChild variant="outline" className="sm:flex-1">
          <Link
            href={`${surface.base}/${encodeURIComponent(business.slug)}${surface.query}`}
          >
            {t("business_page.status.book_again")}
          </Link>
        </Button>
      </div>
    </div>
  );
}

/**
 * Confirmation / status screen of a booking made on the business page. The
 * access token comes from the link's fragment or this device's storage;
 * without it the API answers 404 and the page says so.
 */
export function ReservationStatusPage({
  slug,
  uuid,
  initialBusiness,
}: {
  slug: string;
  uuid: string;
  initialBusiness?: PublicBusiness;
}) {
  const { t } = useLocale();
  // undefined on the server render; null when this device has no token.
  const token = useSyncExternalStore(
    noop,
    () => readAccessToken(uuid),
    () => undefined,
  );
  const returned = useSyncExternalStore(noop, readPaymentReturn, () => null);
  // Embedded screen: watching the payment opened in another tab.
  const [watchUntil, setWatchUntil] = useState<number | null>(null);
  const businessQuery = usePublicBusiness(slug, initialBusiness);
  const reservation = usePublicReservation(
    slug,
    uuid,
    token ?? null,
    returned !== null,
    watchUntil,
  );
  const business = businessQuery.data;

  let body: React.ReactNode;
  if (token === null && returned !== null) {
    // Back from the provider in the tab the embedded screen opened: the
    // result shows in the booking's own window.
    body = (
      <EmptyState
        title={t("business_page.status.payment_returned_title")}
        description={t("business_page.status.payment_returned_description")}
        action={
          <Button variant="outline" onClick={() => window.close()}>
            {t("business_page.status.close_tab")}
          </Button>
        }
      />
    );
  } else if (token === null) {
    body = (
      <EmptyState
        title={t("business_page.status.no_access_title")}
        description={t("business_page.status.no_access_description")}
        action={
          <Button asChild variant="outline">
            <CustomerPanelLink>
              {t("business_page.my_bookings")}
            </CustomerPanelLink>
          </Button>
        }
      />
    );
  } else if (reservation.isError) {
    const notFound =
      isApiError(reservation.error) && reservation.error.status === 404;
    body = (
      <ErrorState
        title={
          notFound
            ? t("business_page.status.no_access_title")
            : t("business_page.load_failed")
        }
        description={
          notFound ? t("business_page.status.no_access_description") : undefined
        }
        onRetry={notFound ? undefined : () => void reservation.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  } else if (!reservation.data || !business || !token) {
    body = <Skeleton className="h-72 w-full rounded-xl" />;
  } else {
    body = (
      <ReservationView
        reservation={reservation.data}
        business={business}
        token={token}
        returned={returned}
        onPaymentOpened={() => setWatchUntil(Date.now() + PAYMENT_TAB_WATCH_MS)}
      />
    );
  }

  return (
    <BusinessPageShell slug={slug} business={business}>
      {body}
    </BusinessPageShell>
  );
}
