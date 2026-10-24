"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { Loading } from "@/components/common/loading";
import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { THEME_FONT_LABEL_KEYS } from "@/features/business-page/lib/theme";
import {
  FIELD_LABEL_KEYS,
  INVOICE_ITEM_LABEL_KEYS,
  SALES_CHANNEL_LABEL_KEYS,
} from "@/features/tenant-settings/constants";
import { useSettingsVersionPair } from "@/features/tenant-settings/hooks/use-tenant-settings";
import {
  diffSettings,
  type SettingsDiffRow,
} from "@/features/tenant-settings/lib/settings-diff";
import type {
  InvoiceItemType,
  SalesChannelCode,
} from "@/features/tenant-settings/types";
import { useLocale } from "@/providers/locale-provider";

const MAPPING_FIELD_KEYS: Record<string, string> = {
  account_code: "tenant_settings.fields.account_code",
  vat_rate: "tenant_settings.fields.vat_rate",
  description: "tenant_settings.fields.mapping_description",
};

const ENUM_VALUE_KEYS: Record<string, string> = {
  "reservation.approval_mode:auto": "tenant_settings.approval_modes.auto",
  "reservation.approval_mode:staff": "tenant_settings.approval_modes.staff",
  ...Object.fromEntries(
    Object.entries(THEME_FONT_LABEL_KEYS).map(([font, key]) => [
      `theme.font:${font}`,
      key,
    ]),
  ),
};

/**
 * Read-only diff of two stored versions: every changed field with its value
 * in the older and the newer version.
 */
export function VersionDiffDialog({
  versions,
  onOpenChange,
}: {
  /** [older, newer] version numbers; null closes the dialog. */
  versions: [number, number] | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const [olderQuery, newerQuery] = useSettingsVersionPair(versions);
  const older = olderQuery?.data;
  const newer = newerQuery?.data;
  const loading = Boolean(olderQuery?.isLoading || newerQuery?.isLoading);
  const failed = Boolean(olderQuery?.isError || newerQuery?.isError);

  const rows = useMemo(
    () => (older && newer ? diffSettings(older.settings, newer.settings) : []),
    [newer, older],
  );

  const pathLabel = (path: string) => {
    const known = FIELD_LABEL_KEYS[path];
    if (known) return t(known);
    const [section, a, b, c] = path.split(".");
    if (section === "sales_channels") {
      return `${t("tenant_settings.tabs.channels")} · ${t(
        SALES_CHANNEL_LABEL_KEYS[a as SalesChannelCode] ?? a,
      )}`;
    }
    if (section === "tax" && a === "invoice_item_mappings") {
      return `${t(INVOICE_ITEM_LABEL_KEYS[b as InvoiceItemType] ?? b)} · ${t(
        MAPPING_FIELD_KEYS[c] ?? c,
      )}`;
    }
    if (section === "booking" && a === "fields") {
      return `${t("tenant_settings.booking.fields_title")} · ${b}`;
    }
    return path;
  };

  const display = (path: string, value: string) => {
    if (value === "") return "—";
    if (value === "true") return t("tenant_settings.diff.yes");
    if (value === "false") return t("tenant_settings.diff.no");
    const enumKey = ENUM_VALUE_KEYS[`${path}:${value}`];
    return enumKey ? t(enumKey) : value;
  };

  const columns = [
    createColumn<SettingsDiffRow>({
      accessorKey: "path",
      labelKey: "tenant_settings.diff.field",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <span className="font-medium">{pathLabel(row.original.path)}</span>
      ),
    }),
    createColumn<SettingsDiffRow>({
      accessorKey: "before",
      labelKey: "tenant_settings.diff.before",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="text-muted-foreground break-words whitespace-pre-wrap line-through decoration-1">
          {display(row.original.path, row.original.before)}
        </span>
      ),
    }),
    createColumn<SettingsDiffRow>({
      accessorKey: "after",
      labelKey: "tenant_settings.diff.after",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="break-words whitespace-pre-wrap">
          {display(row.original.path, row.original.after)}
        </span>
      ),
    }),
  ] as ColumnDef<SettingsDiffRow, unknown>[];

  return (
    <Dialog open={Boolean(versions)} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {versions
              ? t("tenant_settings.diff.title", {
                  older: versions[0],
                  newer: versions[1],
                })
              : ""}
          </DialogTitle>
          <DialogDescription>
            {t("tenant_settings.diff.description")}
          </DialogDescription>
        </DialogHeader>
        {loading ? (
          <Loading label={t("common.loading")} />
        ) : failed ? (
          <p className="text-destructive text-sm">
            {t("tenant_settings.diff.load_failed")}
          </p>
        ) : rows.length === 0 ? (
          <p className="text-muted-foreground py-6 text-center text-sm">
            {t("tenant_settings.diff.no_changes")}
          </p>
        ) : (
          <EntityTable
            columns={columns}
            data={rows}
            getRowId={(row) => row.path}
            emptyTitle={t("tenant_settings.diff.no_changes")}
            manual={{ filtering: false, sorting: false, pagination: false }}
            features={{
              persistKey: "tenant-settings-version-diff-v1",
              rowSelection: false,
              columnFilters: false,
              facetedFilters: false,
              sorting: false,
              pagination: false,
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
