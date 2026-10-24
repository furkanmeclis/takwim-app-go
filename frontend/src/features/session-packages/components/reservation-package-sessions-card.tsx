"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ticket, Trash2 } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { AppForm, AppSelect } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useNavModules } from "@/features/nav-engine/lib/modules";
import {
  useApplyReservationPackage,
  useRemoveReservationPackageSession,
  useReservationPackageOptions,
} from "@/features/reservations/hooks/use-reservations";
import type { Reservation } from "@/features/reservations/types";
import {
  PACKAGES_MODULE,
  RELEASE_REASON_LABEL_KEYS,
  SESSION_STATUS_LABEL_KEYS,
  SESSION_STATUS_TONES,
} from "@/features/session-packages/constants";
import type {
  ReservationPackageOption,
  ReservationPackageSession,
} from "@/features/session-packages/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { amount, date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const FORM_ID = "reservation-package-form";

/** Visit states whose package sessions may still change. */
const OPEN_STATUSES = ["requested", "active"];

/**
 * Package sessions of the visit (TEC-920): a line paid by a session of the
 * customer's package costs nothing on the visit (the package was paid when
 * it was sold). The session is held until the visit is completed (then it
 * is used and its value is the line's revenue), and goes back to the
 * package when the visit is cancelled, rejected, expires or is a no-show.
 */
export function ReservationPackageSessionsCard({
  reservation,
  editable,
}: {
  reservation: Reservation;
  editable: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const hasModule = useNavModules();
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<ReservationPackageSession | null>(
    null,
  );
  const remove = useRemoveReservationPackageSession(reservation.uuid);
  const sessions = reservation.package_sessions ?? [];
  const canChange = editable && OPEN_STATUSES.includes(reservation.status);
  const canOpenPackage = can(permissions.tenant.sessionPackages.read);

  const lineName = useMemo(() => {
    const names = new Map(
      reservation.lines.map((line) => [line.index, line.service_name]),
    );
    return (index: number) => names.get(index) ?? `#${index + 1}`;
  }, [reservation.lines]);

  const columns = useMemo(
    () =>
      [
        createColumn<ReservationPackageSession>({
          id: "line",
          accessorFn: (row) => row.line_index,
          labelKey: "session_packages.reservation.columns.line",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {lineName(row.original.line_index)}
            </span>
          ),
        }),
        createColumn<ReservationPackageSession>({
          id: "package",
          accessorFn: (row) => row.customer_package.name,
          labelKey: "session_packages.reservation.columns.package",
          enableSorting: false,
          cell: ({ row }) => {
            const s = row.original;
            const label = t("session_packages.reservation.session_of", {
              name: s.customer_package.name,
              no: s.session_no,
              total: s.total_sessions,
            });
            return canOpenPackage ? (
              <Link
                className="text-primary hover:underline"
                href={routes.tenant.packages.detail(
                  slug,
                  s.customer_package.uuid,
                )}
                onClick={(event) => event.stopPropagation()}
              >
                {label}
              </Link>
            ) : (
              label
            );
          },
        }),
        createColumn<ReservationPackageSession>({
          accessorKey: "amount",
          labelKey: "session_packages.reservation.columns.amount",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {amount(row.original.amount, reservation.currency, locale)}
            </span>
          ),
        }),
        createColumn<ReservationPackageSession>({
          accessorKey: "status",
          labelKey: "session_packages.reservation.columns.status",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => {
            const s = row.original;
            const reason = s.release_reason
              ? t(
                  RELEASE_REASON_LABEL_KEYS[s.release_reason] ??
                    "session_packages.release_reasons.other",
                )
              : "";
            const when = s.consumed_at ?? s.released_at;
            return (
              <div className="flex flex-col items-start gap-1">
                <StatusChip
                  label={t(SESSION_STATUS_LABEL_KEYS[s.status])}
                  tone={SESSION_STATUS_TONES[s.status]}
                />
                {reason || when ? (
                  <span className="text-muted-foreground text-xs">
                    {[reason, when ? datetime(when, undefined, locale) : ""]
                      .filter(Boolean)
                      .join(" · ")}
                  </span>
                ) : null}
              </div>
            );
          },
        }),
        ...(canChange
          ? [
              createColumn<ReservationPackageSession>({
                id: "actions",
                labelKey: "reservations.columns.actions",
                enableSorting: false,
                enableHiding: false,
                cell: ({ row }) =>
                  row.original.status === "held" ? (
                    <EntityRowActions
                      actions={[
                        {
                          id: "remove",
                          label: t("session_packages.reservation.remove"),
                          icon: Trash2,
                          variant: "destructive",
                          onSelect: () => setRemoving(row.original),
                        },
                      ]}
                    />
                  ) : null,
              }),
            ]
          : []),
      ] as ColumnDef<ReservationPackageSession, unknown>[],
    [
      canChange,
      canOpenPackage,
      lineName,
      locale,
      reservation.currency,
      slug,
      t,
    ],
  );

  if (!hasModule(PACKAGES_MODULE)) return null;
  const live = sessions.filter((s) => s.status !== "released");

  return (
    <EntitySectionCard
      title={t("session_packages.reservation.title")}
      badge={live.length}
      actions={
        canChange ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setAdding(true)}
          >
            <Ticket className="size-4" />
            {t("session_packages.reservation.add")}
          </Button>
        ) : null
      }
    >
      <p className="text-muted-foreground mb-3 text-sm">
        {t("session_packages.reservation.hint")}
      </p>
      <EntityTable
        columns={columns}
        data={sessions}
        getRowId={(row) => row.uuid}
        emptyTitle={t("session_packages.reservation.empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "tenant-reservation-package-sessions-v1",
        }}
      />

      {canChange ? (
        <ApplyPackageDialog
          reservation={reservation}
          lineName={lineName}
          open={adding}
          onOpenChange={setAdding}
        />
      ) : null}
      <ConfirmDialog
        open={removing !== null}
        title={t("session_packages.reservation.remove_title")}
        description={
          removing
            ? t("session_packages.reservation.remove_description", {
                name: removing.customer_package.name,
                line: lineName(removing.line_index),
              })
            : undefined
        }
        confirmLabel={t("session_packages.reservation.remove")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setRemoving(null)}
        onConfirm={() => {
          if (!removing) return;
          remove.mutate(
            {
              sessionUuid: removing.uuid,
              body: { expected_version: reservation.version },
            },
            { onSettled: () => setRemoving(null) },
          );
        }}
      />
    </EntitySectionCard>
  );
}

function createApplySchema(t: (key: string) => string) {
  return z.object({
    customer_package_uuid: z
      .string()
      .min(1, t("session_packages.validation.package_required")),
    line_index: z
      .string()
      .min(1, t("session_packages.validation.line_required")),
  });
}

type ApplyFormValues = z.infer<ReturnType<typeof createApplySchema>>;

function ApplyFields({
  options,
  lineName,
  currency,
}: {
  options: ReservationPackageOption[];
  lineName: (index: number) => string;
  currency: string;
}) {
  const { t, locale } = useLocale();
  const { control } = useFormContext<ApplyFormValues>();
  const picked = useWatch({ control, name: "customer_package_uuid" });
  const option = options.find((o) => o.customer_package_uuid === picked);

  return (
    <>
      <AppSelect
        name="customer_package_uuid"
        label={t("session_packages.reservation.fields.package")}
        placeholder={t(
          "session_packages.reservation.fields.package_placeholder",
        )}
        options={options.map((o) => ({
          value: o.customer_package_uuid,
          label: t("session_packages.reservation.option", {
            name: o.name,
            remaining: o.remaining,
            total: o.total_sessions,
          }),
        }))}
        description={
          option
            ? t("session_packages.reservation.fields.package_hint", {
                value: amount(option.next_value, currency, locale),
                expires: option.expires_on
                  ? date(option.expires_on, undefined, locale)
                  : t("session_packages.validity_none"),
              })
            : undefined
        }
      />
      <AppSelect
        name="line_index"
        label={t("session_packages.reservation.fields.line")}
        placeholder={t("session_packages.reservation.fields.line_placeholder")}
        options={(option?.line_indexes ?? []).map((index) => ({
          value: String(index),
          label: lineName(index),
        }))}
      />
    </>
  );
}

function ApplyPackageDialog({
  reservation,
  lineName,
  open,
  onOpenChange,
}: {
  reservation: Reservation;
  lineName: (index: number) => string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const apply = useApplyReservationPackage(reservation.uuid);
  const options = useReservationPackageOptions(reservation.uuid, open);
  const schema = useMemo(() => createApplySchema(t), [t]);
  const items = options.data?.items ?? [];
  const only = items.length === 1 ? items[0] : undefined;

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("session_packages.reservation.dialog.title")}
      description={t("session_packages.reservation.dialog.description", {
        name: reservation.customer.name,
      })}
      className="sm:max-w-lg"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={FORM_ID}
            disabled={apply.isPending || items.length === 0}
          >
            {t("session_packages.reservation.dialog.submit")}
          </Button>
        </>
      }
    >
      {open ? (
        options.isSuccess && items.length === 0 ? (
          <Alert>
            <AlertDescription>
              {t("session_packages.reservation.dialog.none")}
            </AlertDescription>
          </Alert>
        ) : (
          <AppForm<ApplyFormValues>
            key={`${reservation.uuid}-${reservation.version}-${options.isSuccess}`}
            id={FORM_ID}
            className="grid gap-4"
            schema={schema}
            defaultValues={{
              customer_package_uuid: only?.customer_package_uuid ?? "",
              line_index:
                only && only.line_indexes.length === 1
                  ? String(only.line_indexes[0])
                  : "",
            }}
            onSubmit={async (values) => {
              await apply.mutateAsync({
                customer_package_uuid: values.customer_package_uuid,
                line_index: Number(values.line_index),
                expected_version: reservation.version,
              });
              onOpenChange(false);
            }}
          >
            <ApplyFields
              options={items}
              lineName={lineName}
              currency={reservation.currency}
            />
          </AppForm>
        )
      ) : null}
    </AppDialog>
  );
}
