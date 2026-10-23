"use client";

import { ArrowRight } from "lucide-react";
import { useMemo } from "react";

import { Loading } from "@/components/common/loading";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  FIELD_LABEL_KEYS,
  INVOICE_ITEM_LABEL_KEYS,
  SALES_CHANNEL_LABEL_KEYS,
} from "@/features/tenant-settings/constants";
import { useSettingsVersionPair } from "@/features/tenant-settings/hooks/use-tenant-settings";
import { diffSettings } from "@/features/tenant-settings/lib/settings-diff";
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
  "payment.prepayment_rule:none": "tenant_settings.prepayment_rules.none",
  "payment.prepayment_rule:fixed": "tenant_settings.prepayment_rules.fixed",
  "payment.prepayment_rule:percent": "tenant_settings.prepayment_rules.percent",
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
    return path;
  };

  const display = (path: string, value: string) => {
    if (value === "") return "—";
    if (value === "true") return t("tenant_settings.diff.yes");
    if (value === "false") return t("tenant_settings.diff.no");
    const enumKey = ENUM_VALUE_KEYS[`${path}:${value}`];
    return enumKey ? t(enumKey) : value;
  };

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
          <ScrollArea className="max-h-[60vh] pr-3">
            <table className="w-full text-sm">
              <thead className="text-muted-foreground text-left text-xs">
                <tr>
                  <th className="py-2 pr-3 font-medium">
                    {t("tenant_settings.diff.field")}
                  </th>
                  <th className="py-2 pr-3 font-medium">
                    <Badge variant="outline">v{versions?.[0]}</Badge>
                  </th>
                  <th className="w-4" />
                  <th className="py-2 font-medium">
                    <Badge variant="secondary">v{versions?.[1]}</Badge>
                  </th>
                </tr>
              </thead>
              <tbody className="divide-border divide-y">
                {rows.map((row) => (
                  <tr key={row.path} className="align-top">
                    <td className="py-2 pr-3 font-medium">
                      {pathLabel(row.path)}
                    </td>
                    <td className="text-muted-foreground py-2 pr-3 break-words whitespace-pre-wrap line-through decoration-1">
                      {display(row.path, row.before)}
                    </td>
                    <td className="text-muted-foreground py-2">
                      <ArrowRight className="size-3.5" />
                    </td>
                    <td className="py-2 break-words whitespace-pre-wrap">
                      {display(row.path, row.after)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </ScrollArea>
        )}
      </DialogContent>
    </Dialog>
  );
}
