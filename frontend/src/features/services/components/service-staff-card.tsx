"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityDeleteDialog,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { ServiceStaffDialog } from "@/features/services/components/service-staff-dialog";
import { useSetServiceStaff } from "@/features/services/hooks/use-services";
import { priceLabel } from "@/features/services/lib/format";
import {
  normalizePrice,
  nullableInt,
  type StaffLinkFormValues,
} from "@/features/services/schemas/service-form";
import type {
  Service,
  ServiceStaff,
  ServiceStaffRequest,
} from "@/features/services/types";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type StaffInput = ServiceStaffRequest["staff"][number];

function toInput(link: ServiceStaff): StaffInput {
  return {
    staff_uuid: link.staff_uuid,
    price: link.price,
    duration_minutes: link.duration_minutes,
  };
}

/** Staff who may perform the service, with optional personal price / duration. */
export function ServiceStaffCard({ service }: { service: Service }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.services.write);
  const save = useSetServiceStaff(service.uuid);
  const staff = service.staff ?? [];
  const [dialog, setDialog] = useState<{
    open: boolean;
    link: ServiceStaff | null;
  }>({ open: false, link: null });
  const [removeTarget, setRemoveTarget] = useState<ServiceStaff | null>(null);

  const put = (links: StaffInput[]) => save.mutateAsync({ staff: links });

  const submit = async (values: StaffLinkFormValues) => {
    const next: StaffInput = {
      staff_uuid: values.staff_uuid,
      price: normalizePrice(values.price),
      duration_minutes: nullableInt(values.duration_minutes),
    };
    await put([
      ...staff
        .filter((link) => link.staff_uuid !== values.staff_uuid)
        .map(toInput),
      next,
    ]);
  };

  const columns = [
    createColumn<ServiceStaff>({
      accessorKey: "name",
      labelKey: "services.staff.fields.staff",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <span className="flex items-center gap-2 font-medium">
          <span
            aria-hidden
            className="size-2.5 rounded-full"
            style={{ backgroundColor: row.original.color || undefined }}
          />
          {row.original.name}
          {row.original.title ? (
            <span className="text-muted-foreground text-xs font-normal">
              {row.original.title}
            </span>
          ) : null}
        </span>
      ),
    }),
    createColumn<ServiceStaff>({
      id: "price",
      accessorFn: (row) => row.price ?? service.price,
      labelKey: "services.staff.fields.price",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) =>
        row.original.price != null
          ? priceLabel(row.original.price, service.currency, locale)
          : t("services.staff.same_as_service"),
    }),
    createColumn<ServiceStaff>({
      id: "duration",
      accessorFn: (row) => row.duration_minutes ?? service.duration_minutes,
      labelKey: "services.staff.fields.duration",
      enableSorting: false,
      cell: ({ row }) =>
        row.original.duration_minutes != null
          ? t("services.minutes", {
              minutes: String(row.original.duration_minutes),
            })
          : t("services.staff.same_as_service"),
    }),
    createColumn<ServiceStaff>({
      id: "is_active",
      accessorFn: (row) => String(row.is_active),
      labelKey: "services.columns.is_active",
      enableSorting: false,
      cell: ({ row }) => (
        <StatusChip
          label={
            row.original.is_active
              ? t("services.status.active")
              : t("services.status.inactive")
          }
          tone={row.original.is_active ? "success" : "default"}
        />
      ),
    }),
    createColumn<ServiceStaff>({
      id: "actions",
      labelKey: "services.columns.actions",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <EntityRowActions
          actions={[
            {
              id: "edit",
              label: t("services.actions.edit"),
              icon: Pencil,
              permission: permissions.tenant.services.write,
              onSelect: () => setDialog({ open: true, link: row.original }),
            },
            {
              id: "remove",
              label: t("services.staff.remove"),
              icon: Trash2,
              variant: "destructive",
              permission: permissions.tenant.services.write,
              onSelect: () => setRemoveTarget(row.original),
            },
          ]}
        />
      ),
    }),
  ] as ColumnDef<ServiceStaff, unknown>[];

  return (
    <EntitySectionCard
      title={t("services.detail.staff")}
      badge={staff.length}
      actions={
        canWrite ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setDialog({ open: true, link: null })}
          >
            <Plus className="size-4" />
            {t("services.staff.add")}
          </Button>
        ) : null
      }
    >
      {service.staff_mode === "none" ? (
        <p className="text-muted-foreground mb-3 text-sm">
          {t("services.staff.not_used")}
        </p>
      ) : null}
      <EntityTable
        columns={columns}
        data={staff}
        getRowId={(row) => row.staff_uuid}
        emptyTitle={t("services.staff.empty_title")}
        emptyDescription={t("services.staff.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "tenant-service-staff-v1",
        }}
      />
      <ServiceStaffDialog
        open={dialog.open}
        link={dialog.link}
        linkedUuids={staff.map((link) => link.staff_uuid)}
        catalogPrice={service.price}
        catalogDuration={service.duration_minutes}
        isPending={save.isPending}
        onOpenChange={(open) =>
          setDialog((prev) => ({ open, link: open ? prev.link : null }))
        }
        onSubmit={submit}
      />
      <EntityDeleteDialog
        open={Boolean(removeTarget)}
        softDelete={false}
        entityLabel={removeTarget?.name ?? ""}
        isPending={save.isPending}
        onCancel={() => setRemoveTarget(null)}
        onConfirm={() => {
          if (!removeTarget) return;
          void put(
            staff
              .filter((link) => link.staff_uuid !== removeTarget.staff_uuid)
              .map(toInput),
          ).finally(() => setRemoveTarget(null));
        }}
      />
    </EntitySectionCard>
  );
}
