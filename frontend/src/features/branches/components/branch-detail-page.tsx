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
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { BranchActivityCard } from "@/features/branches/components/branch-activity-card";
import { BranchGalleryCard } from "@/features/branches/components/branch-gallery-card";
import { ServicePointsCard } from "@/features/branches/components/service-points-card";
import {
  HolidayExceptionsView,
  WorkingHoursView,
} from "@/features/branches/components/working-hours-view";
import {
  useBranch,
  useDeleteBranch,
  useUpdateBranch,
} from "@/features/branches/hooks/use-branches";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Show page: details, working hours, service points, activity. */
export function BranchDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useBranch(uuid);
  const update = useUpdateBranch();
  const remove = useDeleteBranch();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const branch = query.data;
  const title = branch?.name ?? t("branches.detail_title");

  const coordinates =
    branch && branch.latitude != null && branch.longitude != null
      ? `${branch.latitude}, ${branch.longitude}`
      : null;

  return (
    <EntityPage
      title={title}
      description={t("branches.detail_description")}
      permission={permissions.tenant.branches.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("branches.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("branches.title"), href: routes.tenant.branches.root(slug) },
        { label: title },
      ]}
      actions={
        branch ? (
          <PermissionGuard permission={permissions.tenant.branches.write}>
            <EntityActions>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={update.isPending}
                onClick={() =>
                  update.mutate({
                    uuid,
                    body: { is_active: !branch.is_active },
                  })
                }
              >
                {branch.is_active ? (
                  <PowerOff className="size-4" />
                ) : (
                  <Power className="size-4" />
                )}
                {branch.is_active
                  ? t("branches.actions.deactivate")
                  : t("branches.actions.activate")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className="size-4" />
                {t("branches.actions.delete")}
              </Button>
              <Button asChild size="sm">
                <Link href={routes.tenant.branches.edit(slug, uuid)}>
                  <Pencil className="size-4" />
                  {t("branches.actions.edit")}
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
          description={t("branches.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {branch ? (
        <div className="space-y-6">
          <EntityHeader
            title={branch.name}
            subtitle={
              <code className="bg-muted rounded px-1.5 py-0.5 text-xs font-semibold">
                {branch.code}
              </code>
            }
            badges={
              <StatusChip
                label={
                  branch.is_active
                    ? t("branches.status.active")
                    : t("branches.status.inactive")
                }
                tone={branch.is_active ? "success" : "default"}
              />
            }
            description={
              [branch.district, branch.city].filter(Boolean).join(", ") ||
              undefined
            }
          />

          <div className="grid gap-6 lg:grid-cols-3">
            <EntitySectionCard
              title={t("branches.detail.details")}
              className="lg:col-span-2"
            >
              <EntityDetail
                sections={[
                  {
                    id: "details",
                    fields: [
                      {
                        key: "code",
                        label: t("branches.fields.code"),
                        value: branch.code,
                      },
                      {
                        key: "name",
                        label: t("branches.fields.name"),
                        value: branch.name,
                      },
                      {
                        key: "slug",
                        label: t("branches.fields.slug"),
                        value: branch.slug,
                      },
                      {
                        key: "show_online",
                        label: t("branches.fields.show_online"),
                        value: branch.show_online
                          ? t("branches.online.visible")
                          : t("branches.online.hidden"),
                      },
                      {
                        key: "timezone",
                        label: t("branches.fields.timezone"),
                        value: branch.timezone
                          ? branch.effective_timezone
                          : t("branches.fields.timezone_business", {
                              zone: branch.effective_timezone,
                            }),
                      },
                      {
                        key: "description",
                        label: t("branches.fields.description"),
                        value: branch.description || "—",
                      },
                      {
                        key: "address",
                        label: t("branches.fields.address"),
                        value: branch.address || "—",
                      },
                      {
                        key: "city",
                        label: t("branches.fields.city"),
                        value:
                          [branch.district, branch.city]
                            .filter(Boolean)
                            .join(", ") || "—",
                      },
                      {
                        key: "phone",
                        label: t("branches.fields.phone"),
                        value: branch.phone ? (
                          <a href={`tel:${branch.phone}`} className="underline">
                            {branch.phone}
                          </a>
                        ) : (
                          "—"
                        ),
                      },
                      {
                        key: "email",
                        label: t("branches.fields.email"),
                        value: branch.email ? (
                          <a
                            href={`mailto:${branch.email}`}
                            className="underline"
                          >
                            {branch.email}
                          </a>
                        ) : (
                          "—"
                        ),
                      },
                      {
                        key: "coordinates",
                        label: t("branches.fields.coordinates"),
                        value: coordinates ?? "—",
                      },
                      {
                        key: "prep_minutes",
                        label: t("branches.fields.prep_minutes"),
                        value: t("branches.minutes", {
                          count: branch.prep_minutes,
                        }),
                      },
                      {
                        key: "created_at",
                        label: t("branches.fields.created_at"),
                        value: datetime(branch.created_at, undefined, locale),
                      },
                      {
                        key: "updated_at",
                        label: t("branches.fields.updated_at"),
                        value: datetime(branch.updated_at, undefined, locale),
                      },
                    ],
                  },
                ]}
              />
            </EntitySectionCard>

            <EntitySectionCard title={t("branches.detail.working_hours")}>
              <div className="space-y-5">
                <WorkingHoursView hours={branch.working_hours} />
                <div className="space-y-2">
                  <h3 className="text-sm font-semibold">
                    {t("branches.detail.holidays")}
                  </h3>
                  <HolidayExceptionsView holidays={branch.holiday_exceptions} />
                </div>
              </div>
            </EntitySectionCard>
          </div>

          <BranchGalleryCard branchUuid={uuid} images={branch.images ?? []} />

          <ServicePointsCard
            branchUuid={uuid}
            branchHours={branch.working_hours}
          />
          <BranchActivityCard branchUuid={uuid} />

          <EntityDeleteDialog
            open={confirmDelete}
            entityLabel={`${branch.code} · ${branch.name}`}
            isPending={remove.isPending}
            onCancel={() => setConfirmDelete(false)}
            onConfirm={() =>
              remove.mutate(uuid, {
                onSuccess: () => router.push(routes.tenant.branches.root(slug)),
                onSettled: () => setConfirmDelete(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
