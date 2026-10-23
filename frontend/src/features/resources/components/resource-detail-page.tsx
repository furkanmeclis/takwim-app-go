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
import { WorkingHoursView } from "@/features/branches/components/working-hours-view";
import { ResourceActivityCard } from "@/features/resources/components/resource-activity-card";
import { ResourceClosuresCard } from "@/features/resources/components/resource-closures-card";
import {
  useDeleteResource,
  useResource,
  useUpdateResource,
} from "@/features/resources/hooks/use-resources";
import {
  capacityLabel,
  useResourceKinds,
} from "@/features/resources/lib/kinds";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Show page: identity, capacity, hours, closed periods, activity. */
export function ResourceDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const kinds = useResourceKinds();
  const query = useResource(uuid);
  const update = useUpdateResource();
  const remove = useDeleteResource();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const resource = query.data;
  const title = resource?.name ?? t("resources.detail_title");
  const hours = resource?.working_hours ?? resource?.branch_hours;

  return (
    <EntityPage
      title={title}
      description={t("resources.detail_description")}
      permission={permissions.tenant.resources.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("resources.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("resources.title"),
          href: routes.tenant.resources.root(slug),
        },
        { label: title },
      ]}
      actions={
        resource ? (
          <PermissionGuard permission={permissions.tenant.resources.write}>
            <EntityActions>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={update.isPending}
                onClick={() =>
                  update.mutate({
                    uuid,
                    body: { is_active: !resource.is_active },
                  })
                }
              >
                {resource.is_active ? (
                  <PowerOff className="size-4" />
                ) : (
                  <Power className="size-4" />
                )}
                {resource.is_active
                  ? t("resources.actions.deactivate")
                  : t("resources.actions.activate")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className="size-4" />
                {t("resources.actions.delete")}
              </Button>
              <Button asChild size="sm">
                <Link href={routes.tenant.resources.edit(slug, uuid)}>
                  <Pencil className="size-4" />
                  {t("resources.actions.edit")}
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
          description={t("resources.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {resource ? (
        <div className="space-y-6">
          <EntityHeader
            title={resource.name}
            subtitle={[resource.branch.name, resource.zone]
              .filter(Boolean)
              .join(" · ")}
            badges={
              <>
                <Badge variant="secondary">{kinds.label(resource.kind)}</Badge>
                <StatusChip
                  label={
                    resource.is_active
                      ? t("resources.status.active")
                      : t("resources.status.inactive")
                  }
                  tone={resource.is_active ? "success" : "default"}
                />
                <StatusChip
                  label={
                    resource.show_online
                      ? t("resources.online.visible")
                      : t("resources.online.hidden")
                  }
                  tone={resource.show_online ? "success" : "default"}
                />
              </>
            }
            description={resource.description || undefined}
          />

          <EntitySectionCard title={t("resources.detail.overview")}>
            <EntityDetail
              sections={[
                {
                  id: "overview",
                  fields: [
                    {
                      key: "branch",
                      label: t("resources.fields.branch"),
                      value: `${resource.branch.name} (${resource.branch.code})`,
                    },
                    {
                      key: "zone",
                      label: t("resources.fields.zone"),
                      value: resource.zone || "—",
                    },
                    {
                      key: "capacity",
                      label: t("resources.fields.capacity"),
                      value: t("resources.capacity_people", {
                        range: capacityLabel(
                          resource.min_capacity,
                          resource.capacity,
                        ),
                      }),
                    },
                    ...(resource.kind === "table"
                      ? [
                          {
                            key: "combinable",
                            label: t("resources.fields.combinable"),
                            value: resource.combinable
                              ? t("resources.combinable.yes")
                              : t("resources.combinable.no"),
                          },
                        ]
                      : []),
                    {
                      key: "sort_order",
                      label: t("resources.fields.sort_order"),
                      value: String(resource.sort_order),
                    },
                    {
                      key: "created_at",
                      label: t("resources.fields.created_at"),
                      value: datetime(resource.created_at, undefined, locale),
                    },
                    {
                      key: "updated_at",
                      label: t("resources.fields.updated_at"),
                      value: datetime(resource.updated_at, undefined, locale),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <EntitySectionCard
            title={t("resources.detail.hours")}
            badge={
              resource.working_hours
                ? t("resources.hours.own")
                : t("resources.hours.branch")
            }
          >
            {hours ? (
              <WorkingHoursView hours={hours} />
            ) : (
              <p className="text-muted-foreground text-sm">
                {t("resources.hours.unknown")}
              </p>
            )}
          </EntitySectionCard>

          <ResourceClosuresCard resourceUuid={uuid} />
          <ResourceActivityCard resourceUuid={uuid} />

          <EntityDeleteDialog
            open={confirmDelete}
            entityLabel={resource.name}
            isPending={remove.isPending}
            onCancel={() => setConfirmDelete(false)}
            onConfirm={() =>
              remove.mutate(uuid, {
                onSuccess: () =>
                  router.push(routes.tenant.resources.root(slug)),
                onSettled: () => setConfirmDelete(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
