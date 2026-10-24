"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { FlaskConical, Loader2 } from "lucide-react";
import Link from "next/link";

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
import { BusinessPageShell } from "@/features/business-page/components/business-page-shell";
import { reportBookingError } from "@/features/business-page/lib/errors";
import { publicBookingService } from "@/features/business-page/services/public-booking.service";
import type { TestPaymentPage as Page } from "@/features/business-page/types";
import { priceLabel } from "@/features/services/lib/format";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const statusPath = (page: Page, outcome?: "done" | "failed") =>
  `/b/${encodeURIComponent(page.slug)}/reservations/${encodeURIComponent(
    page.reservation_uuid,
  )}${outcome ? `?payment=${outcome}` : ""}`;

/**
 * Hosted page of the development payment provider (`testpay`): what a real
 * provider's page would be. Paying or declining makes the server send the
 * signed notification a provider would send, then returns to the booking's
 * status page like a provider's return link.
 */
export function TestPaymentPage({ paymentRef }: { paymentRef: string }) {
  const { t, locale } = useLocale();
  const query = useQuery({
    queryKey: ["test-payment", paymentRef],
    queryFn: () => publicBookingService.testPayment(paymentRef),
    retry: false,
  });
  const complete = useMutation({
    mutationFn: (outcome: "succeeded" | "failed") =>
      publicBookingService.completeTestPayment(paymentRef, outcome),
  });
  const page = query.data;

  const finish = (outcome: "succeeded" | "failed") => {
    if (!page) return;
    complete.mutate(outcome, {
      onSuccess: () =>
        window.location.assign(
          statusPath(page, outcome === "succeeded" ? "done" : "failed"),
        ),
      onError: (error) => reportBookingError(error, t),
    });
  };
  const busy = complete.isPending || complete.isSuccess;

  let body: React.ReactNode;
  if (query.isError) {
    const notFound = isApiError(query.error) && query.error.status === 404;
    body = notFound ? (
      <EmptyState title={t("business_page.test_payment.not_found")} />
    ) : (
      <ErrorState
        title={t("business_page.load_failed")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  } else if (!page) {
    body = <Skeleton className="h-72 w-full rounded-xl" />;
  } else {
    body = (
      <div className="space-y-4">
        <Alert>
          <FlaskConical />
          <AlertTitle>{t("business_page.test_payment.title")}</AlertTitle>
          <AlertDescription>
            {t("business_page.test_payment.description")}
          </AlertDescription>
        </Alert>
        <Card className="gap-3 py-4">
          <CardHeader className="px-4">
            <CardTitle>{page.business}</CardTitle>
            <CardDescription>
              {t("business_page.test_payment.reservation", {
                number: page.reservation_number,
              })}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 px-4">
            <p className="flex justify-between text-sm">
              <span className="text-muted-foreground">
                {t("business_page.test_payment.amount")}
              </span>
              <span className="font-semibold tabular-nums">
                {priceLabel(page.amount, page.currency, locale)}
              </span>
            </p>
            <p className="flex justify-between text-sm">
              <span className="text-muted-foreground">
                {t("business_page.test_payment.expires")}
              </span>
              <span className="tabular-nums">
                {datetime(page.expires_at, undefined, locale)}
              </span>
            </p>
            <Separator />
            {page.status === "pending" ? (
              <div className="flex flex-col gap-2 sm:flex-row">
                <Button
                  className="sm:flex-1"
                  disabled={busy}
                  onClick={() => finish("succeeded")}
                >
                  {busy ? <Loader2 className="animate-spin" /> : null}
                  {t("business_page.test_payment.pay")}
                </Button>
                <Button
                  variant="outline"
                  className="sm:flex-1"
                  disabled={busy}
                  onClick={() => finish("failed")}
                >
                  {t("business_page.test_payment.decline")}
                </Button>
              </div>
            ) : (
              <p className="text-muted-foreground text-sm">
                {t("business_page.test_payment.closed")}
              </p>
            )}
            <Button asChild variant="ghost" className="w-full">
              <Link href={statusPath(page)}>
                {t("business_page.test_payment.back")}
              </Link>
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <BusinessPageShell slug={page?.slug ?? ""}>{body}</BusinessPageShell>;
}
