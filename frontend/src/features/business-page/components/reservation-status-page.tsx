"use client";

import {
  CalendarCheck,
  CircleCheck,
  CircleX,
  Clock,
  CreditCard,
} from "lucide-react";
import Link from "next/link";
import { useSyncExternalStore } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
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
import { BusinessPageShell } from "@/features/business-page/components/business-page-shell";
import { CUSTOMER_PANEL_PATH } from "@/features/business-page/constants";
import {
  usePublicBusiness,
  usePublicReservation,
} from "@/features/business-page/hooks/use-public-booking";
import { readAccessToken } from "@/features/business-page/lib/access";
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
import { useLocale } from "@/providers/locale-provider";

const noop = () => () => {};

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

function ReservationView({
  reservation: r,
  business,
}: {
  reservation: PublicReservation;
  business: PublicBusiness;
}) {
  const { t, locale } = useLocale();
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
            {r.branch.name} ·{" "}
            {t("business_page.status.number", { number: r.number })}
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
                </span>
                <span className="shrink-0 tabular-nums">
                  {money(line.unit_price)}
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
          <Link href={CUSTOMER_PANEL_PATH}>
            {t("business_page.my_bookings")}
          </Link>
        </Button>
        <Button asChild variant="outline" className="sm:flex-1">
          <Link href={`/b/${encodeURIComponent(business.slug)}`}>
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
  const businessQuery = usePublicBusiness(slug, initialBusiness);
  const reservation = usePublicReservation(slug, uuid, token ?? null);
  const business = businessQuery.data;

  let body: React.ReactNode;
  if (token === null) {
    body = (
      <EmptyState
        title={t("business_page.status.no_access_title")}
        description={t("business_page.status.no_access_description")}
        action={
          <Button asChild variant="outline">
            <Link href={CUSTOMER_PANEL_PATH}>
              {t("business_page.my_bookings")}
            </Link>
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
  } else if (!reservation.data || !business) {
    body = <Skeleton className="h-72 w-full rounded-xl" />;
  } else {
    body = (
      <ReservationView reservation={reservation.data} business={business} />
    );
  }

  return (
    <BusinessPageShell slug={slug} business={business}>
      {body}
    </BusinessPageShell>
  );
}
