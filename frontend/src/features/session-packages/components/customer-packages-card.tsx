"use client";

import { ShoppingBag } from "lucide-react";
import { useState } from "react";

import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { useNavModules } from "@/features/nav-engine/lib/modules";
import { CustomerPackagesTable } from "@/features/session-packages/components/customer-packages-table";
import { SellPackageDialog } from "@/features/session-packages/components/sell-package-dialog";
import { PACKAGES_MODULE } from "@/features/session-packages/constants";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * The customer's packages with their balance (customer page). Hidden
 * without the packages module or `tenant.session_packages.read`.
 */
export function CustomerPackagesCard({
  customer,
}: {
  customer: { uuid: string; name: string };
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const hasModule = useNavModules();
  const [selling, setSelling] = useState(false);

  if (
    !hasModule(PACKAGES_MODULE) ||
    !can(permissions.tenant.sessionPackages.read)
  ) {
    return null;
  }
  const canSell =
    hasModule("cash") &&
    can(permissions.tenant.sessionPackages.sell) &&
    can(permissions.tenant.payment.collect);

  return (
    <EntitySectionCard
      title={t("session_packages.customer_card.title")}
      actions={
        canSell ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setSelling(true)}
          >
            <ShoppingBag className="size-4" />
            {t("session_packages.actions.sell")}
          </Button>
        ) : null
      }
    >
      <CustomerPackagesTable
        customerUuid={customer.uuid}
        persistKey="tenant-customer-detail-packages-v1"
      />
      {canSell ? (
        <SellPackageDialog
          open={selling}
          onOpenChange={setSelling}
          customer={customer}
        />
      ) : null}
    </EntitySectionCard>
  );
}
