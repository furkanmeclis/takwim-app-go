"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CashAccountForm } from "@/features/cash/components/cash-account-form";
import { useCreateCashAccount } from "@/features/cash/hooks/use-cash";
import { useBusinessToday } from "@/features/cash/lib/format";
import {
  accountFormDefaults,
  accountFormToRequest,
} from "@/features/cash/schemas/cash-forms";
import { useSelectedBranch } from "@/features/tenant/hooks/use-selected-branch";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export function CashAccountCreatePage() {
  const { t } = useLocale();
  const { slug, branchScope } = useTenant();
  const router = useRouter();
  const create = useCreateCashAccount();
  const today = useBusinessToday();
  // A branch picked in the header is the natural home of a new account.
  const [selectedBranch] = useSelectedBranch(slug);
  const defaults = accountFormDefaults(undefined, {
    branchUuid: selectedBranch ?? undefined,
    today,
  });

  return (
    <EntityPage
      title={t("cash.create_title")}
      description={t("cash.create_description")}
      permission={permissions.tenant.cash.manage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("cash.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("cash.title"), href: routes.tenant.cash.root(slug) },
        { label: t("cash.create_title") },
      ]}
    >
      <EntityForm>
        <CashAccountForm
          mode="create"
          defaultValues={defaults}
          allowHeadOffice={branchScope === "all"}
          isSubmitting={create.isPending}
          onCancel={() => router.push(routes.tenant.cash.root(slug))}
          onSubmit={async (values) => {
            const created = await create.mutateAsync(
              accountFormToRequest(values, "create", false),
            );
            router.push(routes.tenant.cash.detail(slug, created.uuid));
          }}
        />
      </EntityForm>
    </EntityPage>
  );
}
