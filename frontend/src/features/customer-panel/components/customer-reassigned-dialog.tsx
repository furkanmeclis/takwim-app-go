"use client";

import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { useMemo, useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { EntityTable } from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useReportReassigned } from "@/features/customer-panel/hooks/use-customer-panel";
import { customerErrorMessage } from "@/features/customer-panel/lib/errors";
import type { CustomerBusiness } from "@/features/customer-panel/types";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * "This number was recently given to me": the person ticks the businesses
 * whose visits are their own; every other business shows only what is
 * booked from now on, and the profile's other devices (the previous
 * holder's) are signed out. Nothing is deleted at the businesses.
 */
export function CustomerReassignedDialog({
  open,
  onOpenChange,
  businesses,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  businesses: CustomerBusiness[];
}) {
  const { t } = useLocale();
  const report = useReportReassigned();
  const [selection, setSelection] = useState<RowSelectionState>({});
  const [error, setError] = useState<string | null>(null);

  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<CustomerBusiness>(),
        createColumn<CustomerBusiness>({
          accessorKey: "name",
          labelKey: "customer_panel.profile.business",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<CustomerBusiness>({
          id: "known_as",
          accessorFn: (row) => `${row.first_name} ${row.last_name}`.trim(),
          labelKey: "customer_panel.profile.known_as",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            `${row.original.first_name} ${row.original.last_name}`.trim() ||
            "—",
        }),
        createColumn<CustomerBusiness>({
          accessorKey: "reservation_count",
          labelKey: "customer_panel.profile.visits",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {row.original.reservation_count}
            </span>
          ),
        }),
      ] as ColumnDef<CustomerBusiness, unknown>[],
    [],
  );

  const close = (next: boolean) => {
    if (!next) {
      setSelection({});
      setError(null);
    }
    onOpenChange(next);
  };

  const submit = () => {
    setError(null);
    const keep = Object.keys(selection).filter((id) => selection[id]);
    report.mutate(keep, {
      onSuccess: () => {
        appToast.success(t("customer_panel.reassigned.done"));
        close(false);
      },
      onError: (err) => setError(customerErrorMessage(t, err)),
    });
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={close}
      title={t("customer_panel.reassigned.title")}
      description={t("customer_panel.reassigned.description")}
      className="sm:max-w-2xl"
      footer={
        <>
          <Button variant="outline" onClick={() => close(false)}>
            {t("customer_panel.actions.keep")}
          </Button>
          <Button
            variant="destructive"
            disabled={report.isPending}
            onClick={submit}
          >
            {t("customer_panel.reassigned.submit")}
          </Button>
        </>
      }
    >
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      <p className="text-muted-foreground text-sm">
        {t("customer_panel.reassigned.keep_hint")}
      </p>
      <EntityTable
        columns={columns}
        data={businesses}
        getRowId={(row) => row.uuid}
        state={{ rowSelection: selection, onRowSelectionChange: setSelection }}
        emptyTitle={t("customer_panel.profile.no_businesses")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: true,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "customer-panel-reassigned-v1",
        }}
      />
    </AppDialog>
  );
}
