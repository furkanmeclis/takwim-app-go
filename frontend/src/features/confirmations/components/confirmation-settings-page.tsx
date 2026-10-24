"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, RotateCcw } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityDetail,
  EntityPage,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ConfirmationRuleDialog } from "@/features/confirmations/components/confirmation-rule-dialog";
import {
  ON_NO_ANSWER_KEYS,
  deadlineLabel,
  followUpLabel,
  timingSummary,
  windowLabel,
} from "@/features/confirmations/constants";
import {
  useConfirmationSettings,
  useResetConfirmationRule,
} from "@/features/confirmations/hooks/use-confirmations";
import type {
  ConfirmationBranchRule,
  ConfirmationRule,
  ConfirmationRuleTarget,
} from "@/features/confirmations/types";
import {
  channelLabel,
  offsetLabel,
} from "@/features/customer-notifications/constants";
import { useSector } from "@/features/sectors/hooks/use-sector";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const STATIC_TABLE = {
  manual: { filtering: false, sorting: false, pagination: false },
  features: {
    rowSelection: false,
    columnFilters: false,
    facetedFilters: false,
    sorting: false,
    pagination: false,
  },
} as const;

/** Which rule a reset removes: the business-wide one (null) or a branch's. */
type ResetTarget = { branch: ConfirmationBranchRule["branch"] | null };

/**
 * Randevu teyidi (TEC-921, the confirmation add-on): when and how
 * customers are asked to confirm their booking, the business-wide rule and
 * each branch's own rule. Without the add-on the page says so and links to
 * the plan page.
 */
export function ConfirmationSettingsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { hasModule } = useSector();
  const enabled = hasModule("confirmation");

  return (
    <EntityPage
      title={t("confirmations.title")}
      description={t("confirmations.description")}
      permission={permissions.tenant.settings.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("confirmations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.section_tenant_settings"),
          href: routes.tenant.settings.root(slug),
        },
        { label: t("confirmations.title") },
      ]}
    >
      {enabled ? (
        <ConfirmationSettings />
      ) : (
        <EmptyState
          title={t("confirmations.addon_off.title")}
          description={t("confirmations.addon_off.description")}
          action={
            <Button asChild variant="outline" size="sm">
              <Link href={routes.tenant.settings.plan(slug)}>
                {t("confirmations.addon_off.action")}
              </Link>
            </Button>
          }
        />
      )}
    </EntityPage>
  );
}

function ConfirmationSettings() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.settings.write);
  const settings = useConfirmationSettings();
  const reset = useResetConfirmationRule();
  const [editing, setEditing] = useState<ConfirmationRuleTarget | null>(null);
  const [resetting, setResetting] = useState<ResetTarget | null>(null);

  const data = settings.data;
  const readiness = useMemo(
    () =>
      Object.fromEntries(
        (data?.channels ?? []).map((c) => [c.name, c.ready]),
      ) as Record<string, boolean>,
    [data?.channels],
  );
  const used = new Set(
    [data?.default, ...(data?.branches ?? []).map((b) => b.rule)].flatMap(
      (r) => (r?.enabled ? r.channels : []),
    ),
  );
  const notReady = (data?.channels ?? []).filter(
    (c) => !c.ready && used.has(c.name),
  );

  const columns = useMemo(
    () =>
      [
        createColumn<ConfirmationBranchRule>({
          id: "branch",
          accessorFn: (row) => row.branch.name,
          labelKey: "confirmations.columns.branch",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.branch.name}</span>
          ),
        }),
        createColumn<ConfirmationBranchRule>({
          id: "state",
          accessorFn: (row) => row.rule.enabled,
          labelKey: "confirmations.columns.state",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <div className="flex flex-wrap items-center gap-1.5">
              <RuleStateChip rule={row.original.rule} />
              <Badge variant="outline">
                {t(
                  row.original.custom
                    ? "confirmations.state.custom"
                    : "confirmations.state.follows_default",
                )}
              </Badge>
            </div>
          ),
        }),
        createColumn<ConfirmationBranchRule>({
          id: "channels",
          accessorFn: (row) => row.rule.channels.join(","),
          labelKey: "confirmations.columns.channels",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.rule.channels
              .map((c) => channelLabel(t, c))
              .join(" → ") || "—",
        }),
        createColumn<ConfirmationBranchRule>({
          id: "timing",
          accessorFn: (row) => row.rule.first_request_minutes,
          labelKey: "confirmations.columns.timing",
          enableSorting: false,
          cell: ({ row }) => timingSummary(t, row.original.rule),
        }),
        createColumn<ConfirmationBranchRule>({
          id: "on_no_answer",
          accessorFn: (row) => row.rule.on_no_answer,
          labelKey: "confirmations.columns.on_no_answer",
          enableSorting: false,
          cell: ({ row }) =>
            t(ON_NO_ANSWER_KEYS[row.original.rule.on_no_answer]),
        }),
        createColumn<ConfirmationBranchRule>({
          id: "actions",
          labelKey: "confirmations.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "edit",
                  label: t(
                    canWrite
                      ? "confirmations.actions.edit"
                      : "confirmations.actions.view",
                  ),
                  icon: Pencil,
                  onSelect: () => setEditing(row.original),
                },
                {
                  id: "reset",
                  label: t("confirmations.actions.reset_branch"),
                  icon: RotateCcw,
                  permission: permissions.tenant.settings.write,
                  disabled: !row.original.custom,
                  onSelect: () => setResetting({ branch: row.original.branch }),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<ConfirmationBranchRule, unknown>[],
    [canWrite, t],
  );

  if (settings.isLoading) return <Loading label={t("common.loading")} />;
  if (settings.isError || !data) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        description={t("confirmations.load_failed")}
        onRetry={() => void settings.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  return (
    <div className="space-y-6">
      {notReady.length ? (
        <Alert>
          <AlertTitle>
            {t("confirmations.readiness.title", {
              channels: notReady.map((c) => channelLabel(t, c.name)).join(", "),
            })}
          </AlertTitle>
          <AlertDescription>
            {t("confirmations.readiness.description")}
          </AlertDescription>
        </Alert>
      ) : null}

      <EntitySectionCard
        title={t("confirmations.default.title")}
        actions={
          <div className="flex flex-wrap gap-2">
            {canWrite && data.default_custom ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setResetting({ branch: null })}
              >
                <RotateCcw className="size-4" />
                {t("confirmations.actions.reset_default")}
              </Button>
            ) : null}
            <Button
              type="button"
              size="sm"
              variant={canWrite ? "default" : "outline"}
              onClick={() =>
                setEditing({
                  branch: null,
                  rule: data.default,
                  custom: data.default_custom,
                })
              }
            >
              <Pencil className="size-4" />
              {t(
                canWrite
                  ? "confirmations.actions.edit"
                  : "confirmations.actions.view",
              )}
            </Button>
          </div>
        }
      >
        <p className="text-muted-foreground mb-4 text-sm">
          {t(
            data.default_custom
              ? "confirmations.default.custom_hint"
              : "confirmations.default.template_hint",
          )}
        </p>
        <RuleSummary rule={data.default} />
      </EntitySectionCard>

      <EntitySectionCard
        title={t("confirmations.branches.title")}
        badge={data.branches.length}
      >
        <p className="text-muted-foreground mb-4 text-sm">
          {t("confirmations.branches.hint")}
        </p>
        <EntityTable
          columns={columns}
          data={data.branches}
          getRowId={(row) => row.branch.uuid}
          onRowClick={(row) => setEditing(row)}
          isLoading={settings.isFetching && !data}
          emptyTitle={t("confirmations.branches.empty")}
          {...STATIC_TABLE}
          features={{
            ...STATIC_TABLE.features,
            persistKey: "tenant-confirmation-branches-v1",
            viewMode: true,
          }}
          toolbarExtra={
            <EntityToolbar
              onRefresh={() => void settings.refetch()}
              refreshDisabled={settings.isFetching}
            />
          }
        />
      </EntitySectionCard>

      <ConfirmationRuleDialog
        target={editing}
        limits={data.limits}
        readiness={readiness}
        readOnly={!canWrite}
        onClose={() => setEditing(null)}
      />

      <ConfirmDialog
        open={Boolean(resetting)}
        title={t("confirmations.reset_title")}
        description={
          resetting
            ? resetting.branch
              ? t("confirmations.reset_branch_description", {
                  branch: resetting.branch.name,
                })
              : t("confirmations.reset_default_description")
            : undefined
        }
        confirmLabel={t("confirmations.actions.reset")}
        isPending={reset.isPending}
        onConfirm={() => {
          if (!resetting) return;
          reset.mutate(resetting.branch?.uuid ?? null, {
            onSuccess: () => setResetting(null),
          });
        }}
        onCancel={() => setResetting(null)}
      />
    </div>
  );
}

function RuleStateChip({ rule }: { rule: ConfirmationRule }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(
        rule.enabled ? "confirmations.state.on" : "confirmations.state.off",
      )}
      tone={rule.enabled ? "success" : "default"}
    />
  );
}

function RuleSummary({ rule }: { rule: ConfirmationRule }) {
  const { t } = useLocale();
  return (
    <EntityDetail
      sections={[
        {
          id: "rule",
          fields: [
            {
              key: "state",
              label: t("confirmations.columns.state"),
              value: <RuleStateChip rule={rule} />,
            },
            {
              key: "channels",
              label: t("confirmations.columns.channels"),
              value:
                rule.channels.map((c) => channelLabel(t, c)).join(" → ") || "—",
            },
            {
              key: "first_request",
              label: t("confirmations.editor.first_request"),
              value: offsetLabel(t, rule.first_request_minutes),
            },
            {
              key: "follow_up",
              label: t("confirmations.editor.follow_up"),
              value: followUpLabel(t, rule.follow_up_minutes),
            },
            {
              key: "max_attempts",
              label: t("confirmations.editor.max_attempts"),
              value: t("confirmations.timing.attempts_value", {
                count: rule.max_attempts,
              }),
            },
            {
              key: "deadline",
              label: t("confirmations.editor.deadline"),
              value: deadlineLabel(t, rule.deadline_minutes),
            },
            {
              key: "window",
              label: t("confirmations.editor.window"),
              value: windowLabel(t, rule),
            },
            {
              key: "on_no_answer",
              label: t("confirmations.editor.on_no_answer"),
              value: t(ON_NO_ANSWER_KEYS[rule.on_no_answer]),
            },
          ],
        },
      ]}
    />
  );
}
