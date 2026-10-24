"use client";

import { LogIn, XCircle } from "lucide-react";
import Link from "next/link";
import { useEffect, useState } from "react";

import { AppSidebarLogo } from "@/components/brand";
import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityActions, EntityPage } from "@/components/entity";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { CustomerReservationBody } from "@/features/customer-panel/components/customer-reservation-detail-page";
import {
  customerErrorMessage,
  isConflict,
} from "@/features/customer-panel/lib/errors";
import { useFragmentToken } from "@/features/customer-panel/lib/fragment";
import { customerPanelService } from "@/features/customer-panel/services/customer-panel.service";
import type { CustomerReservation } from "@/features/customer-panel/types";
import { isApiError } from "@/lib/api";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

type State =
  | { kind: "loading" }
  | { kind: "invalid" }
  | { kind: "error" }
  | { kind: "ready"; reservation: CustomerReservation };

/**
 * `/panel/v#token`: one visit opened by its visit link (confirmation and
 * cancel messages). It shows that visit only and can cancel it under the
 * business's rules; it opens no panel session. The person's other visits
 * need a sign-in.
 */
export function CustomerVisitPage() {
  const { t } = useLocale();
  const token = useFragmentToken();
  const [state, setState] = useState<State>({ kind: "loading" });
  const [confirming, setConfirming] = useState(false);
  const [cancelling, setCancelling] = useState(false);

  const load = (value: string) => {
    customerPanelService
      .visit(value)
      .then((reservation) => setState({ kind: "ready", reservation }))
      .catch((error: unknown) =>
        setState({
          kind:
            isApiError(error) && error.code === "LINK_INVALID"
              ? "invalid"
              : "error",
        }),
      );
  };

  useEffect(() => {
    if (token) load(token);
  }, [token]);

  // No token in the address: nothing to open.
  const view: State = token === "" ? { kind: "invalid" } : state;
  const reservation = view.kind === "ready" ? view.reservation : null;

  const runCancel = () => {
    if (!token || !reservation) return;
    setCancelling(true);
    customerPanelService
      .visitCancel(token, reservation.version)
      .then((next) => {
        setState({ kind: "ready", reservation: next });
        appToast.success(t("customer_panel.cancel.done"));
      })
      .catch((error: unknown) => {
        appToast.error(customerErrorMessage(t, error));
        if (isConflict(error)) load(token);
      })
      .finally(() => {
        setCancelling(false);
        setConfirming(false);
      });
  };

  return (
    <div className="flex min-h-svh flex-col">
      <header className="bg-background/80 sticky top-0 z-40 border-b backdrop-blur">
        <div className="mx-auto flex h-14 w-full max-w-5xl items-center gap-2 px-4 md:h-16 md:px-6">
          <Link
            href={routes.customer.login}
            className="flex min-w-0 items-center gap-2"
            aria-label={t("customer_panel.title")}
          >
            <AppSidebarLogo />
          </Link>
          <div className="ms-auto flex items-center gap-1">
            <LocaleSwitch />
            <ThemeSwitch />
          </div>
        </div>
      </header>
      <main className="mx-auto flex w-full max-w-5xl min-w-0 flex-1 flex-col gap-4 px-4 py-4 md:px-6 md:pb-10">
        {view.kind === "loading" ? (
          <Loading label={t("common.loading")} />
        ) : null}
        {view.kind === "invalid" ? (
          <EmptyState
            title={t("customer_panel.visit.invalid_title")}
            description={t("customer_panel.visit.invalid_description")}
            action={
              <Button asChild>
                <Link href={routes.customer.login}>
                  <LogIn className="size-4" />
                  {t("customer_panel.visit.sign_in")}
                </Link>
              </Button>
            }
          />
        ) : null}
        {view.kind === "error" ? (
          <ErrorState
            title={t("customer_panel.errors.generic")}
            onRetry={token ? () => load(token) : undefined}
            retryLabel={t("common.retry")}
          />
        ) : null}
        {reservation ? (
          <EntityPage
            title={`${reservation.business.name} · #${reservation.number}`}
            actions={
              reservation.actions.cancel.allowed ? (
                <EntityActions>
                  <Button
                    size="sm"
                    variant="destructive"
                    onClick={() => setConfirming(true)}
                  >
                    <XCircle className="size-4" />
                    {t("customer_panel.actions.cancel")}
                  </Button>
                </EntityActions>
              ) : null
            }
          >
            <div className="space-y-6">
              <Alert>
                <LogIn className="size-4" />
                <AlertDescription className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <span>{t("customer_panel.visit.only_this")}</span>
                  <Button asChild size="sm" variant="outline">
                    <Link href={routes.customer.login}>
                      {t("customer_panel.visit.sign_in")}
                    </Link>
                  </Button>
                </AlertDescription>
              </Alert>
              <CustomerReservationBody reservation={reservation} />
            </div>
            <ConfirmDialog
              open={confirming}
              title={t("customer_panel.cancel.title")}
              description={t("customer_panel.cancel.description")}
              confirmLabel={t("customer_panel.actions.cancel")}
              cancelLabel={t("customer_panel.actions.keep")}
              variant="destructive"
              isPending={cancelling}
              onConfirm={runCancel}
              onCancel={() => setConfirming(false)}
            />
          </EntityPage>
        ) : null}
      </main>
    </div>
  );
}
