"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { PackagePlus, Trash2 } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { z } from "zod";

import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import {
  AppCombobox,
  AppInput,
  AppForm,
  type ComboboxOption,
} from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { useNavModules } from "@/features/nav-engine/lib/modules";
import {
  useAddReservationProduct,
  useRemoveReservationProduct,
} from "@/features/reservations/hooks/use-reservations";
import { reservationsService } from "@/features/reservations/services/reservations.service";
import type {
  Reservation,
  ReservationProduct,
  ReservationProductOption,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { amount, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const FORM_ID = "reservation-product-form";

/** Most units of one product a visit takes (API limit). */
const MAX_QUANTITY = 99;

const STATUS_TONES = {
  held: "warning",
  delivered: "success",
  released: "default",
} as const;

/** Visit states whose products may still change. */
const OPEN_STATUSES = ["requested", "active", "in_service"];

/** "Şampuan / 500 ml". */
function productName(p: {
  product: { name: string };
  variant: { name: string } | null;
}) {
  return p.variant ? `${p.product.name} / ${p.variant.name}` : p.product.name;
}

/** Combobox value of a stock item (product, or product + variant). */
function optionValue(o: ReservationProductOption) {
  return o.variant_uuid
    ? `${o.product_uuid}:${o.variant_uuid}`
    : o.product_uuid;
}

/**
 * Products of the visit (TEC-919): each line keeps its price and reserves
 * its units in the branch's stock while it is held; completing the visit
 * hands them over (stock sale + product sale line), cancelling or a
 * no-show releases them. Staff with `tenant.products.sell` add and remove
 * products while the visit is open.
 */
export function ReservationProductsCard({
  reservation,
  editable,
}: {
  reservation: Reservation;
  editable: boolean;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const hasModule = useNavModules();
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<ReservationProduct | null>(null);
  const remove = useRemoveReservationProduct(reservation.uuid);

  const canChange =
    editable &&
    hasModule("products") &&
    can(permissions.tenant.products.sell) &&
    OPEN_STATUSES.includes(reservation.status) &&
    reservation.payment_status !== "online_pending";

  const columns = useMemo(
    () =>
      [
        createColumn<ReservationProduct>({
          id: "product",
          accessorFn: (row) => productName(row),
          labelKey: "reservations.products.columns.product",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <p className="truncate font-medium">
                {productName(row.original)}
              </p>
              <p className="text-muted-foreground truncate text-xs">
                {row.original.sku}
              </p>
            </div>
          ),
        }),
        createColumn<ReservationProduct>({
          accessorKey: "quantity",
          labelKey: "reservations.products.columns.quantity",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.quantity}</span>
          ),
        }),
        createColumn<ReservationProduct>({
          accessorKey: "unit_price",
          labelKey: "reservations.products.columns.unit_price",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {amount(row.original.unit_price, reservation.currency, locale)}
            </span>
          ),
        }),
        createColumn<ReservationProduct>({
          accessorKey: "total",
          labelKey: "reservations.products.columns.total",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {amount(row.original.total, reservation.currency, locale)}
            </span>
          ),
        }),
        createColumn<ReservationProduct>({
          id: "staff",
          accessorFn: (row) => row.staff?.name ?? "",
          labelKey: "reservations.products.columns.staff",
          enableSorting: false,
          cell: ({ row }) => row.original.staff?.name ?? "—",
        }),
        createColumn<ReservationProduct>({
          accessorKey: "status",
          labelKey: "reservations.products.columns.status",
          enableSorting: false,
          cell: ({ row }) => {
            const p = row.original;
            const reason = p.release_reason
              ? t(`reservations.products.release_reasons.${p.release_reason}`)
              : "";
            const when = p.delivered_at ?? p.released_at;
            return (
              <div className="flex flex-col items-start gap-1">
                <StatusChip
                  label={t(`reservations.products.status.${p.status}`)}
                  tone={STATUS_TONES[p.status]}
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
              createColumn<ReservationProduct>({
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
                          label: t("reservations.products.remove"),
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
      ] as ColumnDef<ReservationProduct, unknown>[],
    [canChange, locale, reservation.currency, t],
  );

  const live = reservation.products.filter((p) => p.status !== "released");
  const productsTotal = live.reduce((sum, p) => sum + Number(p.total), 0);

  return (
    <EntitySectionCard
      title={t("reservations.detail.products")}
      badge={live.length}
      actions={
        canChange ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setAdding(true)}
          >
            <PackagePlus className="size-4" />
            {t("reservations.products.add")}
          </Button>
        ) : null
      }
    >
      <p className="text-muted-foreground mb-3 text-sm">
        {live.length > 0
          ? t("reservations.products.summary", {
              total: amount(
                productsTotal.toFixed(2),
                reservation.currency,
                locale,
              ),
            })
          : t("reservations.products.hint")}
      </p>
      <EntityTable
        columns={columns}
        data={reservation.products}
        getRowId={(row) => row.uuid}
        emptyTitle={t("reservations.products.empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "tenant-reservation-products-v1",
        }}
      />

      <AddProductDialog
        reservation={reservation}
        open={adding}
        onOpenChange={setAdding}
      />
      <ConfirmDialog
        open={removing !== null}
        title={t("reservations.products.remove_title")}
        description={
          removing
            ? t("reservations.products.remove_description", {
                name: productName(removing),
                quantity: removing.quantity,
              })
            : undefined
        }
        confirmLabel={t("reservations.products.remove")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setRemoving(null)}
        onConfirm={() => {
          if (!removing) return;
          remove.mutate(
            {
              productUuid: removing.uuid,
              body: { expected_version: reservation.version },
            },
            { onSettled: () => setRemoving(null) },
          );
        }}
      />
    </EntitySectionCard>
  );
}

function createProductSchema(
  t: (key: string, params?: Record<string, string | number>) => string,
) {
  return z.object({
    item: z
      .string()
      .min(1, t("reservations.products.validation.item_required")),
    quantity: z
      .string()
      .refine(
        (v) =>
          /^\d+$/.test(v.trim()) && Number(v) >= 1 && Number(v) <= MAX_QUANTITY,
        t("reservations.products.validation.quantity", { max: MAX_QUANTITY }),
      ),
  });
}

type ProductFormValues = z.infer<ReturnType<typeof createProductSchema>>;

function AddProductDialog({
  reservation,
  open,
  onOpenChange,
}: {
  reservation: Reservation;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const add = useAddReservationProduct(reservation.uuid);
  const schema = useMemo(() => createProductSchema(t), [t]);
  const [items, setItems] = useState<Record<string, ReservationProductOption>>(
    {},
  );

  const toOption = useCallback(
    (o: ReservationProductOption): ComboboxOption => {
      const name = o.variant_name
        ? `${o.product_name} / ${o.variant_name}`
        : o.product_name;
      return {
        value: optionValue(o),
        label: name,
        description: t("reservations.products.option", {
          sku: o.sku,
          price: amount(o.price, reservation.currency, locale),
          available: o.available,
        }),
        disabled: o.available < 1,
      };
    },
    [locale, reservation.currency, t],
  );

  const loadOptions = useCallback(
    async (query: string) => {
      const result = await reservationsService.productOptions(
        slug,
        reservation.uuid,
        query.trim() || undefined,
      );
      setItems((prev) => {
        const next = { ...prev };
        for (const o of result.items) next[optionValue(o)] = o;
        return next;
      });
      return result.items.map(toOption);
    },
    [reservation.uuid, slug, toOption],
  );

  const cached = useMemo(
    () => Object.values(items).map(toOption),
    [items, toOption],
  );

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("reservations.products.dialog.title")}
      description={t("reservations.products.dialog.description", {
        branch: reservation.branch.name,
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
          <Button type="submit" form={FORM_ID} disabled={add.isPending}>
            {t("reservations.products.dialog.submit")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<ProductFormValues>
          key={`${reservation.uuid}-${reservation.version}`}
          id={FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{ item: "", quantity: "1" }}
          onSubmit={async (values) => {
            const picked = items[values.item];
            if (!picked) return;
            await add.mutateAsync({
              product_uuid: picked.product_uuid,
              variant_uuid: picked.variant_uuid,
              quantity: Number(values.quantity),
              expected_version: reservation.version,
            });
            onOpenChange(false);
          }}
        >
          <AppCombobox
            name="item"
            label={t("reservations.products.fields.product")}
            description={t("reservations.products.fields.product_hint")}
            loadOptions={loadOptions}
            options={cached}
            placeholder={t("reservations.products.fields.product_placeholder")}
            searchPlaceholder={t("reservations.products.fields.search")}
            emptyText={t("reservations.products.fields.empty")}
          />
          <AppInput
            name="quantity"
            inputMode="numeric"
            label={t("reservations.products.fields.quantity")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
