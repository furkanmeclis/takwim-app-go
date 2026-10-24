"use client";

import { AlertTriangle } from "lucide-react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatsCard } from "@/components/common/stats-card";
import { StatusChip } from "@/components/common/status-chip";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import type { AppLocale } from "@/config/i18n";
import { EntitlementLimitsCard } from "@/features/entitlements/components/entitlement-overview";
import { USAGE_METRIC_LABEL_KEYS } from "@/features/entitlements/constants";
import type { UsageMetric } from "@/features/entitlements/types";
import {
  CHANNEL_STATUS_LABEL_KEYS,
  CHANNEL_STATUS_TONES,
  PAYMENT_STATUS_LABEL_KEYS,
  PAYMENT_STATUS_TONES,
} from "@/features/organizations/constants";
import { useOrganizationOverview } from "@/features/organizations/hooks/use-organizations";
import type {
  OrganizationOverview,
  OverviewAccount,
  OverviewChannel,
  OverviewMonth,
} from "@/features/organizations/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Translate = ReturnType<typeof useLocale>["t"];

const SMALL_TABLE = {
  rowSelection: false,
  columnFilters: false,
  facetedFilters: false,
  globalFilter: false,
  sorting: false,
  pagination: false,
} as const;

const CLIENT_SIDE = { filtering: false, sorting: false, pagination: false };

/** "2026-10" → "Ekim 2026" in the UI locale. */
export function monthLabel(period: string, locale: AppLocale) {
  const [year, month] = period.split("-").map(Number);
  if (!year || !month) return period;
  return new Intl.DateTimeFormat(locale, {
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  }).format(new Date(Date.UTC(year, month - 1, 1)));
}

function useOverview(uuid: string) {
  const { t } = useLocale();
  const query = useOrganizationOverview(uuid);
  const fallback = query.isLoading ? (
    <Loading label={t("common.loading")} />
  ) : query.isError || !query.data ? (
    <ErrorState
      title={t("common.error_generic")}
      onRetry={() => void query.refetch()}
      retryLabel={t("common.retry")}
    />
  ) : null;
  return { data: query.data, fallback };
}

function VolumeCard({ data }: { data: OrganizationOverview }) {
  const { t, locale } = useLocale();
  const max = Math.max(
    1,
    ...data.reservations.monthly.map((m) => m.reservations),
  );
  const columns = [
    createColumn<OverviewMonth>({
      accessorKey: "period",
      labelKey: "organizations.overview.month",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => monthLabel(row.original.period, locale),
    }),
    createColumn<OverviewMonth>({
      accessorKey: "reservations",
      labelKey: "organizations.overview.reservations",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => (
        <div className="flex min-w-32 items-center gap-3">
          <span className="w-10 text-right tabular-nums">
            {row.original.reservations.toLocaleString(locale)}
          </span>
          <Progress
            className="h-2 flex-1"
            value={(row.original.reservations / max) * 100}
          />
        </div>
      ),
    }),
  ];
  return (
    <EntitySectionCard title={t("organizations.overview.volume_title")}>
      <p className="text-muted-foreground mb-3 text-sm">
        {t("organizations.overview.volume_hint")}
      </p>
      <EntityTable
        columns={columns}
        data={[...data.reservations.monthly].reverse()}
        getRowId={(row) => row.period}
        emptyTitle={t("organizations.overview.volume_empty")}
        manual={CLIENT_SIDE}
        features={{ persistKey: "org-overview-volume-v1", ...SMALL_TABLE }}
      />
    </EntitySectionCard>
  );
}

type UsageRow = {
  metric: UsageMetric;
  current: number;
  previous: number;
  status: OverviewChannel["status"];
};

function CommunicationCard({ data }: { data: OrganizationOverview }) {
  const { t, locale } = useLocale();
  const previous = new Map(
    data.previous_usage.map((row) => [row.metric, row.quantity]),
  );
  const rows: UsageRow[] = data.channels.map((channel) => ({
    metric: channel.channel,
    current: channel.usage,
    previous: previous.get(channel.channel) ?? 0,
    status: channel.status,
  }));
  const columns = [
    createColumn<UsageRow>({
      accessorKey: "metric",
      labelKey: "entitlements.metric",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <div className="flex flex-wrap items-center gap-2">
          <span>{t(USAGE_METRIC_LABEL_KEYS[row.original.metric])}</span>
          {row.original.status !== "ready" ? (
            <StatusChip
              tone={CHANNEL_STATUS_TONES[row.original.status]}
              label={t(CHANNEL_STATUS_LABEL_KEYS[row.original.status])}
            />
          ) : null}
        </div>
      ),
    }),
    createColumn<UsageRow>({
      accessorKey: "current",
      labelKey: "organizations.overview.this_month",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => row.original.current.toLocaleString(locale),
    }),
    createColumn<UsageRow>({
      accessorKey: "previous",
      labelKey: "organizations.overview.previous_month",
      enableSorting: false,
      cell: ({ row }) => row.original.previous.toLocaleString(locale),
    }),
  ];
  return (
    <EntitySectionCard title={t("organizations.overview.communication_title")}>
      <p className="text-muted-foreground mb-3 text-sm">
        {t("organizations.overview.communication_hint")}
      </p>
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.metric}
        emptyTitle={t("entitlements.usage_empty")}
        manual={CLIENT_SIDE}
        features={{ persistKey: "org-overview-usage-v1", ...SMALL_TABLE }}
      />
    </EntitySectionCard>
  );
}

function ConfirmationCard({ data }: { data: OrganizationOverview }) {
  const { t, locale } = useLocale();
  const c = data.confirmation;
  const state = !c.sector_offers
    ? {
        tone: "default" as const,
        key: "organizations.overview.confirmation_no_sector",
      }
    : c.active
      ? {
          tone: "success" as const,
          key: "organizations.overview.confirmation_active",
        }
      : {
          tone: "warning" as const,
          key: "organizations.overview.confirmation_not_granted",
        };
  const stats: Array<[string, number]> = [
    ["organizations.overview.confirmation_total", c.total],
    ["organizations.overview.confirmation_confirmed", c.confirmed],
    ["organizations.overview.confirmation_declined", c.declined],
    ["organizations.overview.confirmation_expired", c.expired],
    ["organizations.overview.confirmation_open", c.open],
  ];
  return (
    <EntitySectionCard title={t("organizations.overview.confirmation_title")}>
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <StatusChip tone={state.tone} label={t(state.key)} />
        </div>
        <p className="text-muted-foreground text-sm">
          {t("organizations.overview.confirmation_hint")}
        </p>
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-3">
          {stats.map(([key, value]) => (
            <div key={key} className="rounded-lg border p-3">
              <dt className="text-muted-foreground text-xs">{t(key)}</dt>
              <dd className="text-lg font-semibold tabular-nums">
                {value.toLocaleString(locale)}
              </dd>
            </div>
          ))}
        </dl>
      </div>
    </EntitySectionCard>
  );
}

function notReadyCount(data: OrganizationOverview) {
  return (
    data.channels.filter((c) => c.status !== "ready").length +
    (data.payment.status === "ready" ? 0 : 1)
  );
}

/**
 * "Genel bakış" (İşletme 360, TEC-927): reservation volume, catalogue,
 * plan use, communication use and the Konfirmasyon right side by side.
 */
export function OrganizationOverviewTab({
  uuid,
  onOpenSetup,
}: {
  uuid: string;
  onOpenSetup: () => void;
}) {
  const { t, locale } = useLocale();
  const { data, fallback } = useOverview(uuid);
  if (!data) return fallback;
  const r = data.reservations;
  const pending = notReadyCount(data);
  const n = (value: number) => value.toLocaleString(locale);

  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatsCard
          title={t("organizations.overview.reservations_this_month")}
          value={n(r.this_month)}
          hint={t("organizations.overview.previous_month_hint", {
            count: n(r.previous_month),
            cancelled: n(r.cancelled_this_month),
          })}
        />
        <StatsCard
          title={t("organizations.overview.upcoming")}
          value={n(r.upcoming)}
          hint={t("organizations.overview.total_hint", { count: n(r.total) })}
        />
        <StatsCard
          title={t("organizations.overview.customers")}
          value={n(data.catalogue.customers)}
        />
        <StatsCard
          title={t("organizations.overview.services")}
          value={n(data.catalogue.services)}
        />
      </div>
      {pending > 0 ? (
        <div className="flex flex-col gap-3 rounded-lg border border-amber-500/40 bg-amber-500/5 p-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-start gap-3">
            <AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-600" />
            <p className="text-sm">
              {t("organizations.overview.setup_pending", { count: pending })}
            </p>
          </div>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={onOpenSetup}
          >
            {t("organizations.overview.open_setup")}
          </Button>
        </div>
      ) : null}
      <div className="grid gap-6 lg:grid-cols-2">
        <EntitlementLimitsCard summary={data.entitlements} />
        <ConfirmationCard data={data} />
      </div>
      <div className="grid gap-6 lg:grid-cols-2">
        <VolumeCard data={data} />
        <CommunicationCard data={data} />
      </div>
    </div>
  );
}

type SetupRow = {
  id: string;
  label: string;
  status: string;
  tone: "default" | "success" | "warning" | "danger";
  account: OverviewAccount | null;
  activity: string;
};

const ACCOUNT_MODE_LABEL_KEYS: Record<OverviewAccount["mode"], string> = {
  sandbox: "organizations.setup.mode.sandbox",
  live: "organizations.setup.mode.live",
};

const ACCOUNT_STATUS_LABEL_KEYS: Record<OverviewAccount["status"], string> = {
  active: "organizations.setup.account_status.active",
  disabled: "organizations.setup.account_status.disabled",
};

function accountText(t: Translate, account: OverviewAccount | null) {
  if (!account) return t("organizations.setup.no_account");
  return [
    account.display_name || account.provider,
    account.provider,
    t(ACCOUNT_MODE_LABEL_KEYS[account.mode]),
    t(ACCOUNT_STATUS_LABEL_KEYS[account.status]),
  ].join(" · ");
}

function checkText(
  t: Translate,
  locale: AppLocale,
  account: OverviewAccount | null,
) {
  if (!account || !account.last_checked_at) return "—";
  const result =
    account.last_check_status === "ok"
      ? t("organizations.setup.check_ok")
      : account.last_check_status === "failed"
        ? t("organizations.setup.check_failed")
        : "";
  return [result, datetime(account.last_checked_at, undefined, locale)]
    .filter(Boolean)
    .join(" · ");
}

/**
 * "Kanallar ve ödeme": the set-up state of every communication channel and
 * of online payment (requirements §16: a channel that is not ready stays
 * visible and its sends are never counted as successful).
 */
export function OrganizationSetupTab({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { data, fallback } = useOverview(uuid);
  if (!data) return fallback;
  const n = (value: number) => value.toLocaleString(locale);
  const rows: SetupRow[] = [
    ...data.channels.map((c) => ({
      id: c.channel,
      label: t(USAGE_METRIC_LABEL_KEYS[c.channel]),
      status: t(CHANNEL_STATUS_LABEL_KEYS[c.status]),
      tone: CHANNEL_STATUS_TONES[c.status],
      account: c.account,
      activity: t("organizations.setup.deliveries", {
        sent: n(c.sent),
        failed: n(c.failed),
        skipped: n(c.skipped),
      }),
    })),
    {
      id: "payment",
      label: t("organizations.setup.payment"),
      status: t(PAYMENT_STATUS_LABEL_KEYS[data.payment.status]),
      tone: PAYMENT_STATUS_TONES[data.payment.status],
      account: data.payment.account,
      activity: t("organizations.setup.cash_accounts", {
        count: n(data.payment.online_cash_accounts),
      }),
    },
  ];
  const columns = [
    createColumn<SetupRow>({
      accessorKey: "label",
      labelKey: "organizations.setup.channel",
      enableSorting: false,
      gridPrimary: true,
    }),
    createColumn<SetupRow>({
      accessorKey: "status",
      labelKey: "organizations.setup.status",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => (
        <StatusChip tone={row.original.tone} label={row.original.status} />
      ),
    }),
    createColumn<SetupRow>({
      id: "account",
      labelKey: "organizations.setup.account",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="text-sm">{accountText(t, row.original.account)}</span>
      ),
    }),
    createColumn<SetupRow>({
      id: "check",
      labelKey: "organizations.setup.last_check",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="text-muted-foreground text-sm">
          {checkText(t, locale, row.original.account)}
        </span>
      ),
    }),
    createColumn<SetupRow>({
      accessorKey: "activity",
      labelKey: "organizations.setup.activity",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="text-muted-foreground text-sm">
          {row.original.activity}
        </span>
      ),
    }),
  ];
  return (
    <EntitySectionCard title={t("organizations.setup.title")}>
      <p className="text-muted-foreground mb-3 text-sm">
        {t("organizations.setup.description", {
          period: monthLabel(data.period, locale),
        })}
      </p>
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.id}
        emptyTitle={t("organizations.setup.empty")}
        manual={CLIENT_SIDE}
        features={{ persistKey: "org-setup-v1", ...SMALL_TABLE }}
      />
    </EntitySectionCard>
  );
}
