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
import {
  PAYMENT_MODE_LABEL_KEYS,
  STAFF_MODE_LABEL_KEYS,
} from "@/features/services/constants";
import { ServiceActivityCard } from "@/features/services/components/service-activity-card";
import { ServiceImageCard } from "@/features/services/components/service-image-card";
import { ServiceOffersCard } from "@/features/services/components/service-offers-card";
import { ServiceStaffCard } from "@/features/services/components/service-staff-card";
import {
  useDeleteService,
  useService,
  useUpdateService,
} from "@/features/services/hooks/use-services";
import { priceLabel } from "@/features/services/lib/format";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Show page: catalog values, branch offers, staff, cover image, activity. */
export function ServiceDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const kinds = useResourceKinds();
  const query = useService(uuid);
  const update = useUpdateService();
  const remove = useDeleteService();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const service = query.data;
  const title = service?.name ?? t("services.detail_title");

  return (
    <EntityPage
      title={title}
      description={t("services.detail_description")}
      permission={permissions.tenant.services.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("services.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("services.title"),
          href: routes.tenant.services.root(slug),
        },
        { label: title },
      ]}
      actions={
        service ? (
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
                    body: { is_active: !service.is_active },
                  })
                }
              >
                {service.is_active ? (
                  <PowerOff className="size-4" />
                ) : (
                  <Power className="size-4" />
                )}
                {service.is_active
                  ? t("services.actions.deactivate")
                  : t("services.actions.activate")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className="size-4" />
                {t("services.actions.delete")}
              </Button>
              <Button asChild size="sm">
                <Link href={routes.tenant.services.edit(slug, uuid)}>
                  <Pencil className="size-4" />
                  {t("services.actions.edit")}
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
          description={t("services.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {service ? (
        <div className="space-y-6">
          <EntityHeader
            title={service.name}
            subtitle={service.category?.name ?? t("services.uncategorized")}
            badges={
              <>
                <StatusChip
                  label={
                    service.is_active
                      ? t("services.status.active")
                      : t("services.status.inactive")
                  }
                  tone={service.is_active ? "success" : "default"}
                />
                <StatusChip
                  label={
                    service.show_online
                      ? t("services.online.visible")
                      : t("services.online.hidden")
                  }
                  tone={service.show_online ? "success" : "default"}
                />
              </>
            }
            description={service.description || undefined}
          />

          <EntitySectionCard title={t("services.detail.overview")}>
            <EntityDetail
              sections={[
                {
                  id: "overview",
                  fields: [
                    {
                      key: "duration",
                      label: t("services.fields.duration"),
                      value: t("services.minutes", {
                        minutes: String(service.duration_minutes),
                      }),
                    },
                    {
                      key: "buffers",
                      label: t("services.fields.buffers"),
                      value: t("services.buffers_value", {
                        before: String(service.buffer_before_minutes),
                        after: String(service.buffer_after_minutes),
                      }),
                    },
                    {
                      key: "price",
                      label: t("services.fields.price_short"),
                      value: priceLabel(
                        service.price,
                        service.currency,
                        locale,
                      ),
                    },
                    {
                      key: "payment_mode",
                      label: t("services.fields.payment_mode"),
                      value: t(PAYMENT_MODE_LABEL_KEYS[service.payment_mode]),
                    },
                    {
                      key: "staff_mode",
                      label: t("services.fields.staff_mode"),
                      value: t(STAFF_MODE_LABEL_KEYS[service.staff_mode]),
                    },
                    {
                      key: "resource_kinds",
                      label: t("services.fields.resource_kinds"),
                      value:
                        service.resource_kinds.length > 0 ? (
                          <span className="flex flex-wrap gap-1">
                            {service.resource_kinds.map((kind) => (
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
                      key: "sort_order",
                      label: t("services.fields.sort_order"),
                      value: String(service.sort_order),
                    },
                    {
                      key: "created_at",
                      label: t("services.fields.created_at"),
                      value: datetime(service.created_at, undefined, locale),
                    },
                    {
                      key: "updated_at",
                      label: t("services.fields.updated_at"),
                      value: datetime(service.updated_at, undefined, locale),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <ServiceOffersCard service={service} />
          <ServiceStaffCard service={service} />
          <ServiceImageCard service={service} />
          <ServiceActivityCard serviceUuid={uuid} />

          <EntityDeleteDialog
            open={confirmDelete}
            entityLabel={service.name}
            isPending={remove.isPending}
            onCancel={() => setConfirmDelete(false)}
            onConfirm={() =>
              remove.mutate(uuid, {
                onSuccess: () => router.push(routes.tenant.services.root(slug)),
                onSettled: () => setConfirmDelete(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
