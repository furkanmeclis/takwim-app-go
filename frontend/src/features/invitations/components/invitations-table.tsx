"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { MailPlus, RotateCw, XCircle } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn, type ListRealtimeOptions } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useInvitationsList,
  useResendInvitation,
  useRevokeInvitation,
} from "@/features/invitations/hooks/use-invitations";
import {
  INVITATION_STATUSES,
  type Invitation,
  type InvitationScope,
  type InvitationStatus,
  type ListInvitationsParams,
} from "@/features/invitations/types";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const STATUS_TONE: Record<
  InvitationStatus,
  "success" | "warning" | "danger" | "default"
> = {
  pending: "warning",
  expired: "default",
  accepted: "success",
  revoked: "danger",
};

const STATUS_LABEL_KEYS: Record<InvitationStatus, string> = {
  pending: "invitations.status.pending",
  expired: "invitations.status.expired",
  accepted: "invitations.status.accepted",
  revoked: "invitations.status.revoked",
};

export function InvitationStatusChip({ status }: { status: InvitationStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(STATUS_LABEL_KEYS[status])}
      tone={STATUS_TONE[status]}
    />
  );
}

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function personName(person: { name: string; surname: string; email: string }) {
  return `${person.name} ${person.surname}`.trim() || person.email;
}

/**
 * Invitations of an organization as a DataTable: e-mail, roles, branch
 * access, inviter, expiry and status (faceted), with resend / revoke row
 * actions behind confirmation. Used by the tenant members page ("Davetler")
 * and the platform organization page.
 */
export function InvitationsTable({
  scope,
  canWrite,
  onInvite,
  realtime,
  persistKey,
  enabled = true,
}: {
  scope: InvitationScope;
  canWrite: boolean;
  onInvite?: () => void;
  realtime?: ListRealtimeOptions;
  persistKey: string;
  enabled?: boolean;
}) {
  const { t, locale } = useLocale();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime,
  });
  const [revoking, setRevoking] = useState<Invitation | null>(null);
  const resend = useResendInvitation(scope);
  const revoke = useRevokeInvitation(scope);

  const listParams = useMemo<ListInvitationsParams>(() => {
    const statuses = filterValues(
      listState.columnFilters.find((filter) => filter.id === "status")?.value,
    );
    return {
      ...listState.params,
      status: statuses.length ? statuses.join(",") : undefined,
    };
  }, [listState.columnFilters, listState.params]);
  const listQuery = useInvitationsList(scope, listParams, enabled);

  const columns = useMemo(
    () =>
      [
        createColumn<Invitation>({
          accessorKey: "email",
          labelKey: "invitations.columns.email",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.email}</span>
          ),
        }),
        createColumn<Invitation>({
          id: "roles",
          accessorFn: (row) => row.roles.map((role) => role.name).join(", "),
          labelKey: "invitations.columns.roles",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-wrap gap-1">
              {row.original.roles.map((role) => (
                <Badge
                  key={role.uuid}
                  variant={role.is_template ? "secondary" : "outline"}
                >
                  {tenantRoleLabel(t, role)}
                </Badge>
              ))}
            </div>
          ),
        }),
        createColumn<Invitation>({
          accessorKey: "branch_scope",
          labelKey: "invitations.columns.branch_scope",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.branch_scope === "all" ? (
              t("invitations.branch_scope.all_short")
            ) : (
              <div className="flex flex-wrap gap-1">
                {row.original.branches.map((branch) => (
                  <Badge
                    key={branch.uuid}
                    variant="outline"
                    title={branch.name}
                  >
                    {branch.code}
                  </Badge>
                ))}
              </div>
            ),
        }),
        createColumn<Invitation>({
          id: "invited_by",
          accessorFn: (row) =>
            row.invited_by ? personName(row.invited_by) : "",
          labelKey: "invitations.columns.invited_by",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            row.original.invited_by ? (
              personName(row.original.invited_by)
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Invitation>({
          accessorKey: "expires_at",
          labelKey: "invitations.columns.expires_at",
          enableSorting: true,
          cell: ({ row }) => (
            <time
              dateTime={row.original.expires_at}
              title={datetime(row.original.expires_at, undefined, locale)}
            >
              {relativeDatetime(row.original.expires_at, locale)}
            </time>
          ),
        }),
        createColumn<Invitation>({
          accessorKey: "status",
          labelKey: "invitations.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: INVITATION_STATUSES.map((value) => ({
            value,
            labelKey: STATUS_LABEL_KEYS[value],
            label: t(STATUS_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => (
            <InvitationStatusChip status={row.original.status} />
          ),
        }),
        createColumn<Invitation>({
          accessorKey: "created_at",
          labelKey: "invitations.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<Invitation>({
          id: "actions",
          labelKey: "invitations.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const open =
              row.original.status === "pending" ||
              row.original.status === "expired";
            if (!canWrite || !open) return null;
            const actions: EntityRowAction[] = [
              {
                id: "resend",
                label: t("invitations.actions.resend"),
                icon: RotateCw,
                disabled: resend.isPending,
                onSelect: () => resend.mutate(row.original.uuid),
              },
              {
                id: "revoke",
                label: t("invitations.actions.revoke"),
                icon: XCircle,
                variant: "destructive",
                onSelect: () => setRevoking(row.original),
              },
            ];
            return <EntityRowActions actions={actions} />;
          },
        }),
      ] as ColumnDef<Invitation, unknown>[],
    [canWrite, locale, resend, t],
  );

  const total = listQuery.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters = Boolean(listParams.q) || Boolean(listParams.status);

  let inviteButton: ReactNode = null;
  if (canWrite && onInvite) {
    inviteButton = (
      <Button type="button" size="sm" onClick={onInvite}>
        <MailPlus className="size-4" />
        {t("invitations.actions.invite")}
      </Button>
    );
  }

  return (
    <>
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("invitations.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() => listQuery.refetch())}
        emptyTitle={
          hasFilters
            ? t("invitations.empty_filtered_title")
            : t("invitations.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("invitations.empty_filtered_description")
            : t("invitations.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{ persistKey }}
        toolbarExtra={
          <>
            {inviteButton}
            <EntityToolbar
              onRefresh={() => void listQuery.refetch()}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
      <ConfirmDialog
        open={Boolean(revoking)}
        title={t("invitations.revoke.title")}
        description={t("invitations.revoke.description", {
          email: revoking?.email ?? "",
        })}
        confirmLabel={t("invitations.actions.revoke")}
        variant="destructive"
        isPending={revoke.isPending}
        onCancel={() => setRevoking(null)}
        onConfirm={() => {
          if (!revoking) return;
          revoke.mutate(revoking.uuid, { onSettled: () => setRevoking(null) });
        }}
      />
    </>
  );
}
