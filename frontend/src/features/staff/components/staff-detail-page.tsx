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
import { StaffActivityCard } from "@/features/staff/components/staff-activity-card";
import { StaffAssignmentsCard } from "@/features/staff/components/staff-assignments-card";
import { StaffAvatar } from "@/features/staff/components/staff-avatar";
import { StaffTimeOffCard } from "@/features/staff/components/staff-time-off-card";
import { StaffShareRulesCard } from "@/features/staff-shares";
import {
  useDeleteStaff,
  useStaffMember,
  useUpdateStaff,
} from "@/features/staff/hooks/use-staff";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Show page: profile, branches with shifts, time off, activity. */
export function StaffDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useStaffMember(uuid);
  const update = useUpdateStaff();
  const remove = useDeleteStaff();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const staff = query.data;
  const title = staff?.name ?? t("staff.detail_title");

  return (
    <EntityPage
      title={title}
      description={t("staff.detail_description")}
      permission={permissions.tenant.staff.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("staff.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("staff.title"), href: routes.tenant.staff.root(slug) },
        { label: title },
      ]}
      actions={
        staff ? (
          <PermissionGuard permission={permissions.tenant.staff.write}>
            <EntityActions>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={update.isPending}
                onClick={() =>
                  update.mutate({
                    uuid,
                    body: { is_active: !staff.is_active },
                  })
                }
              >
                {staff.is_active ? (
                  <PowerOff className="size-4" />
                ) : (
                  <Power className="size-4" />
                )}
                {staff.is_active
                  ? t("staff.actions.deactivate")
                  : t("staff.actions.activate")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className="size-4" />
                {t("staff.actions.delete")}
              </Button>
              <Button asChild size="sm">
                <Link href={routes.tenant.staff.edit(slug, uuid)}>
                  <Pencil className="size-4" />
                  {t("staff.actions.edit")}
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
          description={t("staff.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {staff ? (
        <div className="space-y-6">
          <EntityHeader
            leading={
              <StaffAvatar
                name={staff.name}
                color={staff.color}
                className="size-12 text-base"
              />
            }
            title={staff.name}
            subtitle={staff.title || undefined}
            badges={
              <>
                <StatusChip
                  label={
                    staff.is_active
                      ? t("staff.status.active")
                      : t("staff.status.inactive")
                  }
                  tone={staff.is_active ? "success" : "default"}
                />
                <StatusChip
                  label={
                    staff.show_online
                      ? t("staff.online.visible")
                      : t("staff.online.hidden")
                  }
                  tone={staff.show_online ? "success" : "default"}
                />
              </>
            }
            description={staff.bio || undefined}
          />

          <EntitySectionCard title={t("staff.detail.profile")}>
            <EntityDetail
              sections={[
                {
                  id: "profile",
                  fields: [
                    {
                      key: "phone",
                      label: t("staff.fields.phone"),
                      value: staff.phone ? (
                        <a href={`tel:${staff.phone}`} className="underline">
                          {staff.phone}
                        </a>
                      ) : (
                        "—"
                      ),
                    },
                    {
                      key: "email",
                      label: t("staff.fields.email"),
                      value: staff.email ? (
                        <a href={`mailto:${staff.email}`} className="underline">
                          {staff.email}
                        </a>
                      ) : (
                        "—"
                      ),
                    },
                    {
                      key: "account",
                      label: t("staff.fields.account"),
                      value: staff.account
                        ? `${`${staff.account.name} ${staff.account.surname}`.trim()} · ${staff.account.email}`
                        : t("staff.account.none_long"),
                    },
                    {
                      key: "color",
                      label: t("staff.fields.color"),
                      value: (
                        <span className="inline-flex items-center gap-2">
                          <span
                            aria-hidden
                            className="size-3 rounded-full"
                            style={{ backgroundColor: staff.color }}
                          />
                          <code className="text-xs">{staff.color}</code>
                        </span>
                      ),
                    },
                    {
                      key: "created_at",
                      label: t("staff.fields.created_at"),
                      value: datetime(staff.created_at, undefined, locale),
                    },
                    {
                      key: "updated_at",
                      label: t("staff.fields.updated_at"),
                      value: datetime(staff.updated_at, undefined, locale),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <StaffAssignmentsCard
            staffUuid={uuid}
            assignments={staff.assignments ?? []}
          />
          <StaffTimeOffCard staffUuid={uuid} />
          <StaffShareRulesCard staffUuid={uuid} />
          <StaffActivityCard staffUuid={uuid} />

          <EntityDeleteDialog
            open={confirmDelete}
            entityLabel={staff.name}
            isPending={remove.isPending}
            onCancel={() => setConfirmDelete(false)}
            onConfirm={() =>
              remove.mutate(uuid, {
                onSuccess: () => router.push(routes.tenant.staff.root(slug)),
                onSettled: () => setConfirmDelete(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
