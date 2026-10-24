"use client";

import type { ReactNode } from "react";

import { ExportMenu } from "@/features/io/components/export-menu";
import { ImportButton } from "@/features/io/components/import-button";
import type { IoResource, ResourceCapabilities } from "@/features/io/types";
import { permissions } from "@/config/permissions";
import { usePermission } from "@/providers/permission-provider";

type ResourceIOToolbarProps = {
  resource: IoResource;
  query?: Record<string, string | undefined>;
  capabilities?: ResourceCapabilities;
  onImportComplete?: () => void;
};

const EXPORT_PERM: Partial<Record<IoResource, string>> = {
  "platform.users": permissions.users.export,
  "platform.roles": permissions.roles.export,
  "platform.notifications": permissions.notifications.export,
  "platform.activity": permissions.activity.read,
  "platform.organizations": permissions.organizations.export,
  "tenant.branches": permissions.tenant.branches.export,
  "tenant.staff": permissions.tenant.staff.export,
  "tenant.resources": permissions.tenant.resources.export,
  "tenant.services": permissions.tenant.services.export,
  "tenant.products": permissions.tenant.products.export,
  "tenant.customers": permissions.tenant.customers.export,
  "tenant.reservations": permissions.tenant.reservations.export,
  "tenant.cash_accounts": permissions.tenant.cash.export,
  "tenant.cash_movements": permissions.tenant.cash.export,
  "tenant.stock_movements": permissions.tenant.products.export,
  "tenant.finance_entries": permissions.tenant.report.export,
  "tenant.staff_share_lines": permissions.tenant.report.export,
  "tenant.staff_share_payouts": permissions.tenant.report.export,
  "tenant.customer_packages": permissions.tenant.sessionPackages.export,
};

const IMPORT_PERM: Partial<Record<IoResource, string>> = {
  "platform.users": permissions.users.import,
  "platform.roles": permissions.roles.import,
  "tenant.branches": permissions.tenant.branches.import,
  "tenant.staff": permissions.tenant.staff.import,
  "tenant.resources": permissions.tenant.resources.import,
  "tenant.services": permissions.tenant.services.import,
  "tenant.products": permissions.tenant.products.import,
  "tenant.customers": permissions.tenant.customers.import,
  // A count import writes adjustments: the stock permission (API rule).
  "tenant.stock_movements": permissions.tenant.products.stock,
};

export function ResourceIOToolbar({
  resource,
  query,
  capabilities,
  onImportComplete,
}: ResourceIOToolbarProps) {
  const { can } = usePermission();
  const nodes: ReactNode[] = [];

  const exportPerm = EXPORT_PERM[resource];
  if (capabilities?.export && exportPerm && can(exportPerm)) {
    nodes.push(<ExportMenu key="export" resource={resource} query={query} />);
  }

  const importPerm = IMPORT_PERM[resource];
  if (capabilities?.import && importPerm && can(importPerm)) {
    nodes.push(
      <ImportButton
        key="import"
        resource={resource}
        onComplete={onImportComplete}
      />,
    );
  }

  if (!nodes.length) return null;
  return <>{nodes}</>;
}
