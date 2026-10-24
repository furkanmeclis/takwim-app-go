"use client";

import { Pencil, Plus, Trash2, Wand2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatsCard } from "@/components/common/stats-card";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityActions,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
} from "@/components/entity";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { periodLabel } from "@/features/cash/components/account-statements-card";
import { MatchDialog } from "@/features/cash/components/match-dialog";
import { PostLineDialog } from "@/features/cash/components/post-line-dialog";
import { ReasonDialog } from "@/features/cash/components/reason-dialog";
import { StatementDialog } from "@/features/cash/components/statement-dialog";
import { StatementLineDialog } from "@/features/cash/components/statement-line-dialog";
import {
  LINE_STATUS_TONE,
  useStatementLineColumns,
} from "@/features/cash/components/statement-line-columns";
import {
  CASH_STATEMENT_LINES_RESOURCE,
  CASH_STATEMENT_LINE_STATUSES,
} from "@/features/cash/constants";
import {
  useAutoMatchCashStatement,
  useCashAccount,
  useCashStatement,
  useCashStatementLineFacets,
  useCashStatementLines,
  useCashStatementLinesMeta,
  useCashStatementSummary,
  useDeleteCashStatement,
  useIgnoreCashLine,
  useResetCashLine,
  useReverseCashLine,
} from "@/features/cash/hooks/use-cash";
import { amountTone, useCashAmount } from "@/features/cash/lib/format";
import type {
  CashDirection,
  CashStatementLine,
  CashStatementSummary,
  ListCashStatementLinesParams,
} from "@/features/cash/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { cn } from "@/lib/utils";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const FILTER_IDS = ["status", "line_type", "direction"] as const;
const SORT_FIELDS = ["line_date", "amount", "status", "line_no"] as const;

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

function StatusCounts({ summary }: { summary: CashStatementSummary }) {
  const { t } = useLocale();
  const counts: Record<string, number> = {
    pending: summary.pending_count,
    matched: summary.matched_count,
    difference: summary.difference_count,
    posted: summary.posted_count,
    ignored: summary.ignored_count,
  };
  return (
    <div className="flex flex-wrap gap-1.5">
      {CASH_STATEMENT_LINE_STATUSES.map((status) => (
        <StatusChip
          key={status}
          label={`${t(`cash.line_status.${status}`)} ${counts[status] ?? 0}`}
          tone={counts[status] ? LINE_STATUS_TONE[status] : "default"}
        />
      ))}
    </div>
  );
}

/**
 * One statement: the reconciliation summary (book balance against the stated
 * balance, counts by status), the import / export toolbar and every line
 * with its match, post, ignore and reverse actions.
 */
export function StatementDetailPage({
  accountUuid,
  statementUuid,
}: {
  accountUuid: string;
  statementUuid: string;
}) {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const money = useCashAmount();
  const statementQuery = useCashStatement(statementUuid);
  const accountQuery = useCashAccount(accountUuid);
  const summaryQuery = useCashStatementSummary(statementUuid);
  const facetsQuery = useCashStatementLineFacets(statementUuid);
  const metaQuery = useCashStatementLinesMeta();
  const autoMatch = useAutoMatchCashStatement(statementUuid);
  const remove = useDeleteCashStatement();
  const reset = useResetCashLine();
  const ignore = useIgnoreCashLine();
  const reverse = useReverseCashLine();

  const urlDefaults = useUrlListDefaults(FILTER_IDS, SORT_FIELDS);
  const listState = useServerListState({
    initialSort: urlDefaults.initialSort ?? "line_no",
    initialColumnFilters: urlDefaults.initialColumnFilters,
    initialPageSize: 50,
    realtime: tenantLiveList(organization, CASH_STATEMENT_LINES_RESOURCE),
  });

  const listParams = useMemo<ListCashStatementLinesParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    const direction = column("direction");
    return {
      ...listState.params,
      status: joined(column("status")),
      line_type: joined(column("line_type")),
      direction:
        direction.length === 1 ? (direction[0] as CashDirection) : undefined,
    };
  }, [listState.columnFilters, listState.params]);

  const linesQuery = useCashStatementLines(statementUuid, listParams);

  const [editOpen, setEditOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [lineOpen, setLineOpen] = useState(false);
  const [matching, setMatching] = useState<CashStatementLine | null>(null);
  const [posting, setPosting] = useState<CashStatementLine | null>(null);
  const [resetting, setResetting] = useState<CashStatementLine | null>(null);
  const [ignoring, setIgnoring] = useState<CashStatementLine | null>(null);
  const [reversing, setReversing] = useState<CashStatementLine | null>(null);

  const handlers = useMemo(
    () => ({
      onMatch: setMatching,
      onPost: setPosting,
      onReset: setResetting,
      onIgnore: setIgnoring,
      onReverse: setReversing,
    }),
    [],
  );

  const statement = statementQuery.data;
  const account = accountQuery.data;
  const summary = summaryQuery.data;
  const closed = statement?.status === "closed";
  const locked = closed || account?.status !== "active";
  const columns = useStatementLineColumns({
    facets: facetsQuery.data,
    locked,
    handlers,
  });

  const engineQuery = useMemo(
    () => ({
      statement: statementUuid,
      q: listParams.q,
      status: listParams.status,
      line_type: listParams.line_type,
      direction: listParams.direction,
      sort: listParams.sort,
    }),
    [
      listParams.direction,
      listParams.line_type,
      listParams.q,
      listParams.sort,
      listParams.status,
      statementUuid,
    ],
  );

  const total = linesQuery.data?.total ?? 0;
  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 50;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);
  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.status) ||
    Boolean(listParams.line_type) ||
    Boolean(listParams.direction);

  const refresh = useCallback(() => {
    void linesQuery.refetch();
    void facetsQuery.refetch();
    void summaryQuery.refetch();
    void statementQuery.refetch();
  }, [facetsQuery, linesQuery, statementQuery, summaryQuery]);

  // A closed statement (or an archived account) takes no more lines.
  const capabilities = useMemo(() => {
    const caps = metaQuery.data?.capabilities;
    if (!caps) return undefined;
    return locked ? { ...caps, import: false } : caps;
  }, [locked, metaQuery.data?.capabilities]);

  const title = statement?.name ?? t("cash.statements.detail_title");
  const accountHref = routes.tenant.cash.detail(slug, accountUuid);

  return (
    <EntityPage
      title={title}
      description={t("cash.statements.detail_description")}
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
        {
          label: account?.name ?? t("cash.detail_title"),
          href: accountHref,
        },
        { label: title },
      ]}
      actions={
        statement && !closed ? (
          <EntityActions>
            <PermissionGuard permission={permissions.tenant.cash.import}>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={remove.isPending}
                onClick={() => setDeleteOpen(true)}
              >
                <Trash2 className="size-4" />
                {t("cash.actions.delete_statement")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setEditOpen(true)}
              >
                <Pencil className="size-4" />
                {t("cash.actions.edit")}
              </Button>
            </PermissionGuard>
            <PermissionGuard permission={permissions.tenant.cash.write}>
              <Button
                type="button"
                size="sm"
                disabled={autoMatch.isPending || !statement.line_count}
                onClick={() => autoMatch.mutate()}
              >
                <Wand2 className="size-4" />
                {t("cash.actions.auto_match")}
              </Button>
            </PermissionGuard>
          </EntityActions>
        ) : null
      }
    >
      {statementQuery.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : null}
      {statementQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("cash.statements.not_found")}
          onRetry={() => void statementQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {statement ? (
        <div className="space-y-6">
          <EntityHeader
            title={statement.name}
            subtitle={`${statement.account.name} · ${periodLabel(statement, locale)}`}
            badges={
              <>
                <Badge variant="secondary">
                  {t(`cash.statement_sources.${statement.source}`)}
                </Badge>
                <StatusChip
                  label={t(`cash.statement_status.${statement.status}`)}
                  tone={closed ? "default" : "warning"}
                />
                {summary?.reconciled ? (
                  <StatusChip
                    label={t("cash.statement_status.reconciled")}
                    tone="success"
                  />
                ) : null}
              </>
            }
            description={statement.note || undefined}
          >
            {summary ? <StatusCounts summary={summary} /> : null}
          </EntityHeader>

          {closed ? (
            <Alert>
              <AlertTitle>{t("cash.statements.closed_title")}</AlertTitle>
              <AlertDescription>
                {t("cash.statements.closed_description")}
              </AlertDescription>
            </Alert>
          ) : null}

          {summary ? (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <StatsCard
                title={t("cash.statements.summary.book_balance")}
                value={
                  <span
                    className={cn(
                      "tabular-nums",
                      amountTone(summary.book_balance),
                    )}
                  >
                    {money(summary.book_balance, statement.currency)}
                  </span>
                }
                hint={t("cash.statements.summary.book_balance_hint", {
                  date: date(summary.balance_date, undefined, locale),
                })}
              />
              <StatsCard
                title={t("cash.statements.summary.closing_balance")}
                value={
                  <span className="tabular-nums">
                    {summary.closing_balance
                      ? money(summary.closing_balance, statement.currency)
                      : "—"}
                  </span>
                }
                hint={
                  summary.balance_difference != null
                    ? t("cash.statements.summary.balance_difference", {
                        amount: money(
                          summary.balance_difference,
                          statement.currency,
                        ),
                      })
                    : t("cash.statements.summary.closing_balance_hint")
                }
              />
              <StatsCard
                title={t("cash.statements.summary.totals")}
                value={
                  <span className="flex flex-wrap gap-x-3 text-base tabular-nums">
                    <span className={amountTone(summary.total_in)}>
                      +{money(summary.total_in, statement.currency)}
                    </span>
                    <span className={amountTone(`-${summary.total_out}`)}>
                      −{money(summary.total_out, statement.currency)}
                    </span>
                  </span>
                }
                hint={t("cash.statements.summary.totals_hint", {
                  fee: money(summary.total_fee, statement.currency),
                  from: summary.first_date
                    ? date(summary.first_date, undefined, locale)
                    : "—",
                  to: summary.last_date
                    ? date(summary.last_date, undefined, locale)
                    : "—",
                })}
              />
              <StatsCard
                title={t("cash.statements.summary.open")}
                value={
                  <span className="tabular-nums">
                    {summary.pending_count + summary.difference_count}
                  </span>
                }
                hint={t("cash.statements.summary.open_hint", {
                  pending: money(summary.pending_net, statement.currency),
                  difference: money(
                    summary.difference_total,
                    statement.currency,
                  ),
                })}
              />
            </div>
          ) : null}

          <EntitySectionCard
            title={t("cash.statements.lines_title")}
            badge={linesQuery.data ? total : undefined}
            actions={
              !locked ? (
                <PermissionGuard permission={permissions.tenant.cash.import}>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => setLineOpen(true)}
                  >
                    <Plus className="size-4" />
                    {t("cash.actions.add_line")}
                  </Button>
                </PermissionGuard>
              ) : null
            }
          >
            <EntityTable
              columns={columns}
              data={linesQuery.data?.items ?? []}
              getRowId={(row) => row.uuid}
              isLoading={linesQuery.isLoading}
              isError={linesQuery.isError}
              errorDescription={t("cash.lines.load_failed")}
              onRetry={() => void linesQuery.refetch()}
              realtime={listState.realtimeFor(() =>
                Promise.all([
                  linesQuery.refetch(),
                  facetsQuery.refetch(),
                  summaryQuery.refetch(),
                ]),
              )}
              emptyTitle={
                hasFilters
                  ? t("cash.empty_filtered_title")
                  : t("cash.lines.empty_title")
              }
              emptyDescription={
                hasFilters
                  ? t("cash.empty_filtered_description")
                  : t("cash.lines.empty_description")
              }
              pageCount={pageCount}
              state={listState.tableState}
              features={{
                persistKey: "tenant-cash-statement-lines-v1",
                rowSelection: false,
                viewMode: true,
              }}
              toolbarExtra={
                <>
                  <ResourceIOToolbar
                    resource={CASH_STATEMENT_LINES_RESOURCE}
                    query={engineQuery}
                    capabilities={capabilities}
                    importDefaults={{ statement_uuid: statementUuid }}
                    onImportComplete={refresh}
                  />
                  <EntityToolbar
                    onRefresh={refresh}
                    refreshDisabled={linesQuery.isFetching}
                  />
                </>
              }
            />
          </EntitySectionCard>

          {account ? (
            <>
              <StatementDialog
                account={account}
                statement={statement}
                open={editOpen}
                onOpenChange={setEditOpen}
              />
              <StatementLineDialog
                statement={statement}
                open={lineOpen}
                onOpenChange={setLineOpen}
              />
            </>
          ) : null}
          <MatchDialog
            line={matching}
            open={Boolean(matching)}
            onOpenChange={(open) => {
              if (!open) setMatching(null);
            }}
          />
          <PostLineDialog
            line={posting}
            open={Boolean(posting)}
            onOpenChange={(open) => {
              if (!open) setPosting(null);
            }}
          />
          <ConfirmDialog
            open={Boolean(resetting)}
            title={t("cash.reset_line_dialog.title")}
            description={t(
              resetting?.fee_movement_uuid
                ? "cash.reset_line_dialog.description_fee"
                : "cash.reset_line_dialog.description",
            )}
            confirmLabel={t("cash.lines.actions.reset")}
            variant="destructive"
            isPending={reset.isPending}
            onConfirm={() => {
              if (!resetting) return;
              reset.mutate(
                { lineUuid: resetting.uuid },
                { onSuccess: () => setResetting(null) },
              );
            }}
            onCancel={() => setResetting(null)}
          />
          <ReasonDialog
            open={Boolean(ignoring)}
            title={t("cash.ignore_line_dialog.title")}
            description={t("cash.ignore_line_dialog.description")}
            confirmLabel={t("cash.lines.actions.ignore")}
            isPending={ignore.isPending}
            onOpenChange={(open) => {
              if (!open) setIgnoring(null);
            }}
            onConfirm={(reason) =>
              ignore.mutateAsync({ lineUuid: ignoring!.uuid, reason })
            }
          />
          <ReasonDialog
            open={Boolean(reversing)}
            title={t("cash.reverse_line_dialog.title")}
            description={t(
              reversing?.posted_movement?.transfer_group
                ? "cash.reverse_line_dialog.description_transfer"
                : "cash.reverse_line_dialog.description",
            )}
            confirmLabel={t("cash.lines.actions.reverse")}
            isPending={reverse.isPending}
            onOpenChange={(open) => {
              if (!open) setReversing(null);
            }}
            onConfirm={(reason) =>
              reverse.mutateAsync({ lineUuid: reversing!.uuid, reason })
            }
          />
          <ConfirmDialog
            open={deleteOpen}
            title={t("cash.delete_statement_dialog.title")}
            description={t("cash.delete_statement_dialog.description", {
              name: statement.name,
            })}
            confirmLabel={t("cash.actions.delete_statement")}
            variant="destructive"
            isPending={remove.isPending}
            onConfirm={() =>
              remove.mutate(statement.uuid, {
                onSuccess: () => {
                  setDeleteOpen(false);
                  router.replace(accountHref);
                },
              })
            }
            onCancel={() => setDeleteOpen(false)}
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
