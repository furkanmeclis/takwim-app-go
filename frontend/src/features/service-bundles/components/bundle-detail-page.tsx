"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Pencil, Power, PowerOff, Trash2 } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityDeleteDialog,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useResourceKinds } from "@/features/resources/lib/kinds";
import { BundleActivityCard } from "@/features/service-bundles/components/bundle-activity-card";
import { BundleItemsCard } from "@/features/service-bundles/components/bundle-items-card";
import {
  useDeleteServiceBundle,
  useServiceBundle,
  useUpdateServiceBundle,
} from "@/features/service-bundles/hooks/use-service-bundles";
import { priceLabel } from "@/features/services/lib/format";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Show page: price and derived values, contents with the split, branches, activity. */
export function BundleDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const kinds = useResourceKinds();
  const query = useServiceBundle(uuid);
  const update = useUpdateServiceBundle();
  const remove = useDeleteServiceBundle();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const bundle = query.data;
  const title = bundle?.name ?? t("service_bundles.detail_title");
  const money = (amount: string) =>
    priceLabel(amount, bundle?.currency ?? "TRY", locale);

  return (
    <EntityPage
      title={title}
      description={t("service_bundles.detail_description")}
      permission={permissions.tenant.services.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("service_bundles.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("service_bundles.title"),
          href: routes.tenant.serviceBundles.root(slug),
        },
        { label: title },
      ]}
      actions={
        bundle ? (
          <PermissionGuard permission={permissions.tenant.services.write}>
            <EntityActions>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={update.isPending}
                onClick={() =>
                  update.mutate({
                    uuid,
                    body: { is_active: !bundle.is_active },
                  })
                }
              >
                {bundle.is_active ? (
                  <PowerOff className="size-4" />
                ) : (
                  <Power className="size-4" />
                )}
                {bundle.is_active
                  ? t("service_bundles.actions.deactivate")
                  : t("service_bundles.actions.activate")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className="size-4" />
                {t("service_bundles.actions.delete")}
              </Button>
              <Button asChild size="sm">
                <Link href={routes.tenant.serviceBundles.edit(slug, uuid)}>
                  <Pencil className="size-4" />
                  {t("service_bundles.actions.edit")}
                </Link>
              </Button>
            </EntityActions>
          </PermissionGuard>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("service_bundles.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {bundle ? (
        <div className="space-y-6">
          <EntityHeader
            title={bundle.name}
            subtitle={bundle.items.map((item) => item.service_name).join(" + ")}
            badges={
              <>
                <StatusChip
                  label={
                    bundle.is_active
                      ? t("service_bundles.status.active")
                      : t("service_bundles.status.inactive")
                  }
                  tone={bundle.is_active ? "success" : "default"}
                />
                {bundle.is_active && !bundle.bookable ? (
                  <StatusChip
                    label={t("service_bundles.status.not_bookable")}
                    tone="warning"
                  />
                ) : null}
                <StatusChip
                  label={
                    bundle.show_online
                      ? t("service_bundles.online.visible")
                      : t("service_bundles.online.hidden")
                  }
                  tone={bundle.show_online ? "success" : "default"}
                />
              </>
            }
            description={bundle.description || undefined}
          />

          <EntitySectionCard title={t("service_bundles.detail.overview")}>
            <EntityDetail
              sections={[
                {
                  id: "overview",
                  fields: [
                    {
                      key: "price",
                      label: t("service_bundles.fields.price_short"),
                      value: money(bundle.price),
                    },
                    {
                      key: "list_price",
                      label: t("service_bundles.summary.list_price"),
                      value: money(bundle.list_price),
                    },
                    {
                      key: "discount",
                      label: t("service_bundles.summary.discount"),
                      value: money(bundle.discount),
                    },
                    {
                      key: "duration",
                      label: t("service_bundles.summary.duration"),
                      value: t("services.minutes", {
                        minutes: String(bundle.duration_minutes),
                      }),
                    },
                    {
                      key: "staff",
                      label: t("service_bundles.summary.staff"),
                      value: bundle.staff_required
                        ? t("service_bundles.summary.staff_required")
                        : t("service_bundles.summary.staff_optional"),
                    },
                    {
                      key: "resources",
                      label: t("service_bundles.summary.resources"),
                      value:
                        bundle.resource_kinds.length > 0 ? (
                          <span className="flex flex-wrap gap-1">
                            {bundle.resource_kinds.map((kind) => (
                              <Badge key={kind} variant="secondary">
                                {kinds.label(kind)}
                              </Badge>
                            ))}
                          </span>
                        ) : (
                          t("services.no_resources")
                        ),
                    },
                    {
                      key: "branches",
                      label: t("service_bundles.fields.branches"),
                      value:
                        bundle.branches.length > 0 ? (
                          <span className="flex flex-wrap gap-1">
                            {bundle.branches.map((branch) => (
                              <Badge key={branch.uuid} variant="secondary">
                                {branch.name}
                              </Badge>
                            ))}
                          </span>
                        ) : (
                          t("service_bundles.no_branches")
                        ),
                    },
                    {
                      key: "sort_order",
                      label: t("service_bundles.fields.sort_order"),
                      value: String(bundle.sort_order),
                    },
                    {
                      key: "updated_at",
                      label: t("service_bundles.fields.updated_at"),
                      value: datetime(bundle.updated_at, undefined, locale),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <BundleItemsCard bundle={bundle} />
          <BundleActivityCard bundleUuid={uuid} />

          <EntityDeleteDialog
            open={confirmDelete}
            entityLabel={bundle.name}
            isPending={remove.isPending}
            onCancel={() => setConfirmDelete(false)}
            onConfirm={() =>
              remove.mutate(uuid, {
                onSuccess: () =>
                  router.push(routes.tenant.serviceBundles.root(slug)),
                onSettled: () => setConfirmDelete(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
