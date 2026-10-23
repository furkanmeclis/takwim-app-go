"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CashAccountForm } from "@/features/cash/components/cash-account-form";
import {
  useCashAccount,
  useUpdateCashAccount,
} from "@/features/cash/hooks/use-cash";
import { useBusinessToday } from "@/features/cash/lib/format";
import {
  accountFormDefaults,
  accountFormToRequest,
} from "@/features/cash/schemas/cash-forms";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export function CashAccountEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug, branchScope } = useTenant();
  const router = useRouter();
  const query = useCashAccount(uuid);
  const update = useUpdateCashAccount();
  const today = useBusinessToday();
  const account = query.data;
  const title = account
    ? t("cash.edit_title_with_name", { name: account.name })
    : t("cash.edit_title");

  return (
    <EntityPage
      title={title}
      description={t("cash.edit_description")}
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
        {
          label: account?.name ?? t("cash.detail_title"),
          href: routes.tenant.cash.detail(slug, uuid),
        },
        { label: t("cash.edit_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("cash.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {account ? (
        <EntityForm>
          <CashAccountForm
            key={account.updated_at}
            mode="edit"
            defaultValues={accountFormDefaults(account, { today })}
            openingLocked={account.has_activity}
            allowHeadOffice={branchScope === "all"}
            isSubmitting={update.isPending}
            onCancel={() => router.push(routes.tenant.cash.detail(slug, uuid))}
            onSubmit={async (values) => {
              await update.mutateAsync({
                uuid,
                body: accountFormToRequest(
                  values,
                  "edit",
                  account.has_activity,
                ),
              });
              router.push(routes.tenant.cash.detail(slug, uuid));
            }}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}
