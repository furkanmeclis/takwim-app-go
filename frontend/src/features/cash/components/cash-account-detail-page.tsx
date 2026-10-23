"use client";

import Link from "next/link";
import { Archive, ArchiveRestore, ArrowLeftRight, Pencil } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatsCard } from "@/components/common/stats-card";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { AccountClosingsCard } from "@/features/cash/components/account-closings-card";
import { AccountMovementsCard } from "@/features/cash/components/account-movements-card";
import { CashAccountActivityCard } from "@/features/cash/components/cash-account-activity-card";
import { ClosingDialog } from "@/features/cash/components/closing-dialog";
import { MovementDialog } from "@/features/cash/components/movement-dialog";
import { TransferDialog } from "@/features/cash/components/transfer-dialog";
import {
  useCashAccount,
  useUpdateCashAccount,
} from "@/features/cash/hooks/use-cash";
import {
  amountTone,
  kindLabel,
  useCashAmount,
} from "@/features/cash/lib/format";
import type { CashDirection } from "@/features/cash/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { cn } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Show page: balance, details, the account's book, closings, activity. */
export function CashAccountDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const money = useCashAmount();
  const query = useCashAccount(uuid);
  const update = useUpdateCashAccount();
  const [movement, setMovement] = useState<CashDirection | null>(null);
  const [transferOpen, setTransferOpen] = useState(false);
  const [closingOpen, setClosingOpen] = useState(false);
  const account = query.data;
  const title = account?.name ?? t("cash.detail_title");
  const active = account?.status === "active";

  return (
    <EntityPage
      title={title}
      description={t("cash.detail_description")}
      permission={permissions.tenant.cash.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("cash.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("cash.title"), href: routes.tenant.cash.root(slug) },
        { label: title },
      ]}
      actions={
        account ? (
          <EntityActions>
            {active ? (
              <PermissionGuard permission={permissions.tenant.cash.write}>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setTransferOpen(true)}
                >
                  <ArrowLeftRight className="size-4" />
                  {t("cash.actions.transfer")}
                </Button>
              </PermissionGuard>
            ) : null}
            <PermissionGuard permission={permissions.tenant.cash.manage}>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={update.isPending}
                onClick={() =>
                  update.mutate({
                    uuid,
                    body: { status: active ? "archived" : "active" },
                  })
                }
              >
                {active ? (
                  <Archive className="size-4" />
                ) : (
                  <ArchiveRestore className="size-4" />
                )}
                {active
                  ? t("cash.actions.archive")
                  : t("cash.actions.activate")}
              </Button>
              <Button asChild size="sm">
                <Link href={routes.tenant.cash.edit(slug, uuid)}>
                  <Pencil className="size-4" />
                  {t("cash.actions.edit")}
                </Link>
              </Button>
            </PermissionGuard>
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("cash.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {account ? (
        <div className="space-y-6">
          <EntityHeader
            title={account.name}
            subtitle={account.branch?.name ?? t("cash.head_office")}
            badges={
              <>
                <Badge variant="secondary">{kindLabel(t, account.kind)}</Badge>
                <StatusChip
                  label={t(`cash.status.${account.status}`)}
                  tone={active ? "success" : "default"}
                />
              </>
            }
            description={account.note || undefined}
          />

          <div className="grid gap-4 sm:grid-cols-3">
            <StatsCard
              title={t("cash.detail.balance")}
              value={
                <span
                  className={cn("tabular-nums", amountTone(account.balance))}
                >
                  {money(account.balance, account.currency)}
                </span>
              }
              hint={t("cash.detail.balance_hint")}
            />
            <StatsCard
              title={t("cash.fields.opening_balance", {
                currency: account.currency,
              })}
              value={
                <span className="tabular-nums">
                  {money(account.opening_balance, account.currency)}
                </span>
              }
              hint={date(account.opening_date, undefined, locale)}
            />
            <StatsCard
              title={t("cash.columns.last_closed_date")}
              value={
                account.last_closed_date
                  ? date(account.last_closed_date, undefined, locale)
                  : "—"
              }
              hint={
                account.kind === "cash"
                  ? t("cash.detail.closing_hint")
                  : t("cash.detail.no_closing_hint")
              }
            />
          </div>

          <EntitySectionCard title={t("cash.detail.overview")}>
            <EntityDetail
              sections={[
                {
                  id: "overview",
                  fields: [
                    {
                      key: "branch",
                      label: t("cash.fields.branch"),
                      value: account.branch
                        ? `${account.branch.name} (${account.branch.code})`
                        : t("cash.head_office"),
                    },
                    {
                      key: "kind",
                      label: t("cash.fields.kind"),
                      value: kindLabel(t, account.kind),
                    },
                    {
                      key: "currency",
                      label: t("cash.fields.currency"),
                      value: account.currency,
                    },
                    ...(account.kind === "bank"
                      ? [
                          {
                            key: "bank_name",
                            label: t("cash.fields.bank_name"),
                            value: account.bank_name || "—",
                          },
                          {
                            key: "iban",
                            label: t("cash.fields.iban"),
                            value: account.iban || "—",
                          },
                        ]
                      : []),
                    ...(account.kind === "online"
                      ? [
                          {
                            key: "provider",
                            label: t("cash.fields.provider"),
                            value: account.provider || "—",
                          },
                        ]
                      : []),
                    {
                      key: "created_at",
                      label: t("cash.fields.created_at"),
                      value: datetime(account.created_at, undefined, locale),
                    },
                    {
                      key: "updated_at",
                      label: t("cash.fields.updated_at"),
                      value: datetime(account.updated_at, undefined, locale),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <AccountMovementsCard account={account} onPost={setMovement} />
          {account.kind === "cash" ? (
            <AccountClosingsCard
              account={account}
              onCloseDay={() => setClosingOpen(true)}
            />
          ) : null}
          <CashAccountActivityCard accountUuid={uuid} />

          <MovementDialog
            account={account}
            direction={movement ?? "in"}
            open={movement !== null}
            onOpenChange={(open) => {
              if (!open) setMovement(null);
            }}
          />
          <TransferDialog
            fromAccountUuid={uuid}
            open={transferOpen}
            onOpenChange={setTransferOpen}
          />
          <ClosingDialog
            account={account}
            open={closingOpen}
            onOpenChange={setClosingOpen}
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
