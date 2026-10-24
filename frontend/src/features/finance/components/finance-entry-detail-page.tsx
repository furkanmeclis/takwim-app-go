"use client";

import Link from "next/link";
import { PencilLine, Tags, Undo2 } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatsCard } from "@/components/common/stats-card";
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
import { ReasonDialog } from "@/features/cash/components/reason-dialog";
import { amountTone, useCashAmount } from "@/features/cash/lib/format";
import { EntryActivityCard } from "@/features/finance/components/entry-activity-card";
import { EntryDocumentCard } from "@/features/finance/components/entry-document-card";
import {
  ClassifyDialog,
  CorrectDialog,
} from "@/features/finance/components/entry-edit-dialogs";
import {
  useFinanceEntry,
  useReverseFinanceEntry,
} from "@/features/finance/hooks/use-finance";
import { kindLabel, signedAmount } from "@/features/finance/lib/format";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { cn } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Show page of a record: amount, details, links of its history, document, activity. */
export function FinanceEntryDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const money = useCashAmount();
  const query = useFinanceEntry(uuid);
  const reverse = useReverseFinanceEntry();
  const [dialog, setDialog] = useState<
    "classify" | "correct" | "reverse" | null
  >(null);
  const entry = query.data;
  const title = entry
    ? `${kindLabel(t, entry.kind)} · ${entry.category.name}`
    : t("finance.detail_title");
  const live = entry?.entry_type === "entry" && !entry.reversed_by_uuid;

  const link = (target: string | null | undefined) =>
    target ? (
      <Link
        className="text-primary underline-offset-4 hover:underline"
        href={routes.tenant.finance.detail(slug, target)}
      >
        {t("finance.detail.open_record")}
      </Link>
    ) : null;

  return (
    <EntityPage
      title={title}
      description={t("finance.detail_description")}
      permission={permissions.tenant.finance.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("finance.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("finance.title"), href: routes.tenant.finance.root(slug) },
        { label: title },
      ]}
      actions={
        entry && live ? (
          <EntityActions>
            <PermissionGuard permission={permissions.tenant.finance.write}>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setDialog("classify")}
              >
                <Tags className="size-4" />
                {t("finance.actions.classify")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setDialog("correct")}
              >
                <PencilLine className="size-4" />
                {t("finance.actions.correct")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="destructive"
                onClick={() => setDialog("reverse")}
              >
                <Undo2 className="size-4" />
                {t("finance.actions.reverse")}
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
          description={t("finance.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {entry ? (
        <div className="space-y-6">
          <EntityHeader
            title={entry.category.name}
            subtitle={entry.branch?.name ?? t("finance.head_office")}
            badges={
              <>
                <Badge variant={entry.kind === "income" ? "success" : "danger"}>
                  {kindLabel(t, entry.kind)}
                </Badge>
                {entry.entry_type === "reversal" ? (
                  <Badge variant="warning">
                    {t("finance.entry_types.reversal")}
                  </Badge>
                ) : null}
                {entry.reversed_by_uuid ? (
                  <Badge variant="warning">{t("finance.reversed")}</Badge>
                ) : null}
                {entry.replaces_uuid ? (
                  <Badge variant="secondary">{t("finance.correction")}</Badge>
                ) : null}
              </>
            }
            description={entry.description || undefined}
          />

          <div className="grid gap-4 sm:grid-cols-3">
            <StatsCard
              title={t("finance.fields.amount")}
              value={
                <span
                  className={cn(
                    "tabular-nums",
                    amountTone(signedAmount(entry.kind, entry.amount)),
                  )}
                >
                  {money(
                    signedAmount(entry.kind, entry.amount),
                    entry.currency,
                  )}
                </span>
              }
              hint={entry.account.name}
            />
            <StatsCard
              title={t("finance.fields.entry_date")}
              value={date(entry.entry_date, undefined, locale)}
            />
            <StatsCard
              title={t("finance.fields.staff")}
              value={entry.staff?.name ?? "—"}
            />
          </div>

          <EntitySectionCard title={t("finance.detail.overview")}>
            <EntityDetail
              sections={[
                {
                  id: "overview",
                  fields: [
                    {
                      key: "category",
                      label: t("finance.fields.category"),
                      value: entry.category.name,
                    },
                    {
                      key: "branch",
                      label: t("finance.fields.branch"),
                      value: entry.branch?.name ?? t("finance.head_office"),
                    },
                    {
                      key: "account",
                      label: t(
                        entry.kind === "income"
                          ? "finance.fields.account_income"
                          : "finance.fields.account_expense",
                      ),
                      value: (
                        <Link
                          className="text-primary underline-offset-4 hover:underline"
                          href={routes.tenant.cash.detail(
                            slug,
                            entry.account.uuid,
                          )}
                        >
                          {entry.account.name}
                        </Link>
                      ),
                    },
                    {
                      key: "currency",
                      label: t("finance.fields.currency"),
                      value: entry.currency,
                    },
                    ...(entry.reason
                      ? [
                          {
                            key: "reason",
                            label: t("finance.fields.reason"),
                            value: entry.reason,
                          },
                        ]
                      : []),
                    ...(entry.reverses_uuid
                      ? [
                          {
                            key: "reverses",
                            label: t("finance.detail.reverses"),
                            value: link(entry.reverses_uuid),
                          },
                        ]
                      : []),
                    ...(entry.reversed_by_uuid
                      ? [
                          {
                            key: "reversed_by",
                            label: t("finance.detail.reversed_by"),
                            value: link(entry.reversed_by_uuid),
                          },
                        ]
                      : []),
                    ...(entry.replaces_uuid
                      ? [
                          {
                            key: "replaces",
                            label: t("finance.detail.replaces"),
                            value: link(entry.replaces_uuid),
                          },
                        ]
                      : []),
                    ...(entry.replaced_by_uuid
                      ? [
                          {
                            key: "replaced_by",
                            label: t("finance.detail.replaced_by"),
                            value: link(entry.replaced_by_uuid),
                          },
                        ]
                      : []),
                    {
                      key: "created_by",
                      label: t("finance.fields.created_by"),
                      value: entry.created_by
                        ? `${entry.created_by.name} ${entry.created_by.surname}`.trim()
                        : "—",
                    },
                    {
                      key: "created_at",
                      label: t("finance.fields.created_at"),
                      value: datetime(entry.created_at, undefined, locale),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <EntryDocumentCard entry={entry} />
          <EntryActivityCard entryUuid={uuid} />

          <ClassifyDialog
            entry={entry}
            open={dialog === "classify"}
            onOpenChange={(open) => setDialog(open ? "classify" : null)}
          />
          <CorrectDialog
            entry={entry}
            open={dialog === "correct"}
            onOpenChange={(open) => setDialog(open ? "correct" : null)}
          />
          <ReasonDialog
            open={dialog === "reverse"}
            title={t("finance.reverse_dialog.title")}
            description={t("finance.reverse_dialog.description")}
            confirmLabel={t("finance.actions.reverse")}
            isPending={reverse.isPending}
            onOpenChange={(open) => setDialog(open ? "reverse" : null)}
            onConfirm={(reason) => reverse.mutateAsync({ uuid, reason })}
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
