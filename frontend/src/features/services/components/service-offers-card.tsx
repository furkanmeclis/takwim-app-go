"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityDeleteDialog,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { ServiceOfferDialog } from "@/features/services/components/service-offer-dialog";
import { useSetServiceOffers } from "@/features/services/hooks/use-services";
import { priceLabel } from "@/features/services/lib/format";
import {
  normalizePrice,
  nullableInt,
  type OfferFormValues,
} from "@/features/services/schemas/service-form";
import type {
  Service,
  ServiceOffer,
  ServiceOffersRequest,
} from "@/features/services/types";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type OfferInput = ServiceOffersRequest["offers"][number];

function toInput(offer: ServiceOffer): OfferInput {
  return {
    branch_uuid: offer.branch_uuid,
    price: offer.price,
    duration_minutes: offer.duration_minutes,
    is_active: offer.is_active,
  };
}

/**
 * Branches offering the service with their own price / duration. A member
 * edits the branches of their scope only (`tenant.services.offer`); the
 * API keeps the other branches' offers as they are.
 */
export function ServiceOffersCard({ service }: { service: Service }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canOffer = can(permissions.tenant.services.offer);
  const branches = useAccessibleBranches();
  const save = useSetServiceOffers(service.uuid);
  const [dialog, setDialog] = useState<{
    open: boolean;
    offer: ServiceOffer | null;
  }>({ open: false, offer: null });
  const [removeTarget, setRemoveTarget] = useState<ServiceOffer | null>(null);

  const editable = service.offers.filter((offer) => offer.editable);
  const freeBranches = useMemo(() => {
    const taken = new Set(service.offers.map((offer) => offer.branch_uuid));
    return (branches.data?.items ?? [])
      .filter((branch) => !taken.has(branch.uuid))
      .map((branch) => ({
        value: branch.uuid,
        label: `${branch.name} (${branch.code})`,
      }));
  }, [branches.data?.items, service.offers]);

  const put = (offers: OfferInput[]) => save.mutateAsync({ offers });

  const submit = async (values: OfferFormValues) => {
    const next: OfferInput = {
      branch_uuid: values.branch_uuid,
      price: normalizePrice(values.price),
      duration_minutes: nullableInt(values.duration_minutes),
      is_active: values.is_active,
    };
    const rest = editable
      .filter((offer) => offer.branch_uuid !== values.branch_uuid)
      .map(toInput);
    await put([...rest, next]);
  };

  const money = (amount: string) =>
    priceLabel(amount, service.currency, locale);

  const columns = [
    createColumn<ServiceOffer>({
      accessorKey: "branch_name",
      labelKey: "services.offers.fields.branch",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <span className="font-medium">
          {row.original.branch_name}{" "}
          <span className="text-muted-foreground text-xs">
            {row.original.branch_code}
          </span>
        </span>
      ),
    }),
    createColumn<ServiceOffer>({
      accessorKey: "effective_price",
      labelKey: "services.offers.fields.price",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => (
        <span className="flex items-center gap-2">
          {money(row.original.effective_price)}
          {row.original.price != null ? (
            <Badge variant="secondary">{t("services.offers.override")}</Badge>
          ) : null}
        </span>
      ),
    }),
    createColumn<ServiceOffer>({
      accessorKey: "effective_duration_minutes",
      labelKey: "services.offers.fields.duration",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="flex items-center gap-2">
          {t("services.minutes", {
            minutes: String(row.original.effective_duration_minutes),
          })}
          {row.original.duration_minutes != null ? (
            <Badge variant="secondary">{t("services.offers.override")}</Badge>
          ) : null}
        </span>
      ),
    }),
    createColumn<ServiceOffer>({
      id: "is_active",
      accessorFn: (row) => String(row.is_active),
      labelKey: "services.offers.fields.is_active",
      enableSorting: false,
      cell: ({ row }) => (
        <StatusChip
          label={
            row.original.is_active
              ? t("services.offers.open")
              : t("services.offers.paused")
          }
          tone={row.original.is_active ? "success" : "default"}
        />
      ),
    }),
    createColumn<ServiceOffer>({
      id: "actions",
      labelKey: "services.columns.actions",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) =>
        row.original.editable ? (
          <EntityRowActions
            actions={[
              {
                id: "edit",
                label: t("services.actions.edit"),
                icon: Pencil,
                permission: permissions.tenant.services.offer,
                onSelect: () => setDialog({ open: true, offer: row.original }),
              },
              {
                id: "remove",
                label: t("services.offers.remove"),
                icon: Trash2,
                variant: "destructive",
                permission: permissions.tenant.services.offer,
                onSelect: () => setRemoveTarget(row.original),
              },
            ]}
          />
        ) : null,
    }),
  ] as ColumnDef<ServiceOffer, unknown>[];

  return (
    <EntitySectionCard
      title={t("services.detail.offers")}
      badge={service.offers.length}
      actions={
        canOffer ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={freeBranches.length === 0}
            onClick={() => setDialog({ open: true, offer: null })}
          >
            <Plus className="size-4" />
            {t("services.offers.add")}
          </Button>
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={service.offers}
        getRowId={(row) => row.branch_uuid}
        emptyTitle={t("services.offers.empty_title")}
        emptyDescription={t("services.offers.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "tenant-service-offers-v1",
        }}
      />
      <ServiceOfferDialog
        open={dialog.open}
        offer={dialog.offer}
        branchOptions={freeBranches}
        catalogPrice={service.price}
        catalogDuration={service.duration_minutes}
        isPending={save.isPending}
        onOpenChange={(open) =>
          setDialog((prev) => ({ open, offer: open ? prev.offer : null }))
        }
        onSubmit={submit}
      />
      <EntityDeleteDialog
        open={Boolean(removeTarget)}
        softDelete={false}
        entityLabel={removeTarget?.branch_name ?? ""}
        isPending={save.isPending}
        onCancel={() => setRemoveTarget(null)}
        onConfirm={() => {
          if (!removeTarget) return;
          void put(
            editable
              .filter((offer) => offer.branch_uuid !== removeTarget.branch_uuid)
              .map(toInput),
          ).finally(() => setRemoveTarget(null));
        }}
      />
    </EntitySectionCard>
  );
}
