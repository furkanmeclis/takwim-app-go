"use client";

import { Building2, Pencil, Trash2 } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { z } from "zod";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Timeline } from "@/components/common/timeline";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityActions,
  EntityDeleteDialog,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { AppDatePicker, AppForm, FormActions } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { isoToLocalInput, localInputToISO } from "@/lib/datetime/local-input";
import {
  OrganizationAccessChip,
  OrganizationStatusChip,
  ownerLabel,
} from "@/features/organizations/components/organization-cells";
import { PlatformEntitlementsTab } from "@/features/entitlements/components/platform-entitlements-tab";
import { OrganizationMembersTab } from "@/features/organizations/components/organization-members-tab";
import {
  useDeleteOrganization,
  useOrganization,
  useOrganizationActivity,
  useSetOrganizationAccess,
  useSetOrganizationStatus,
} from "@/features/organizations/hooks/use-organizations";
import type {
  OrganizationActivityEntry,
  OrganizationSummary,
} from "@/features/organizations/types";
import { OrganizationAvatar } from "@/features/tenant/components/organization-avatar";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const TABS = [
  "general",
  "members",
  "status",
  "plan",
  "domains",
  "integrations",
  "activity",
] as const;
type Tab = (typeof TABS)[number];

const TAB_LABEL_KEYS: Record<Tab, string> = {
  general: "organizations.tabs.general",
  members: "organizations.tabs.members",
  status: "organizations.tabs.status",
  plan: "organizations.tabs.plan",
  domains: "organizations.tabs.domains",
  integrations: "organizations.tabs.integrations",
  activity: "organizations.tabs.activity",
};

function resolveTab(value: string | undefined): Tab {
  return (TABS as readonly string[]).includes(value ?? "")
    ? (value as Tab)
    : "general";
}

function Forbidden() {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_forbidden")}
      description={t("organizations.forbidden")}
    />
  );
}

function GeneralTab({ org }: { org: OrganizationSummary }) {
  const { t, locale } = useLocale();
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <EntitySectionCard title={t("organizations.detail.identity")}>
        <EntityDetail
          sections={[
            {
              id: "identity",
              fields: [
                {
                  key: "name",
                  label: t("organizations.fields.name"),
                  value: org.name,
                },
                {
                  key: "slug",
                  label: t("organizations.fields.slug"),
                  value: <code className="text-xs">{org.slug}</code>,
                },
                {
                  key: "tenant",
                  label: t("organizations.detail.tenant_url"),
                  value: (
                    <Link
                      className="underline"
                      href={routes.tenant.home(org.slug)}
                    >
                      {routes.tenant.home(org.slug)}
                    </Link>
                  ),
                },
                {
                  key: "uuid",
                  label: "UUID",
                  value: (
                    <code className="text-muted-foreground text-xs">
                      {org.uuid}
                    </code>
                  ),
                },
                {
                  key: "created_at",
                  label: t("organizations.columns.created_at"),
                  value: datetime(org.created_at, undefined, locale),
                },
                {
                  key: "updated_at",
                  label: t("organizations.fields.updated_at"),
                  value: datetime(org.updated_at, undefined, locale),
                },
              ],
            },
          ]}
        />
      </EntitySectionCard>
      <EntitySectionCard title={t("organizations.detail.overview")}>
        <EntityDetail
          sections={[
            {
              id: "overview",
              fields: [
                {
                  key: "owner",
                  label: t("organizations.columns.owner"),
                  value: org.owner
                    ? `${ownerLabel(org.owner)} · ${org.owner.email}`
                    : org.pending_invitation_count > 0
                      ? t("organizations.owner_invited")
                      : "—",
                },
                {
                  key: "members",
                  label: t("organizations.columns.member_count"),
                  value: org.member_count,
                },
                {
                  key: "invitations",
                  label: t("organizations.detail.pending_invitations"),
                  value: org.pending_invitation_count,
                },
                {
                  key: "branches",
                  label: t("organizations.columns.branch_count"),
                  value: org.branch_count,
                },
              ],
            },
          ]}
        />
      </EntitySectionCard>
    </div>
  );
}

function createAccessSchema(t: (key: string) => string) {
  return z
    .object({ access_starts_at: z.string(), access_ends_at: z.string() })
    .superRefine((values, ctx) => {
      if (!values.access_starts_at) {
        ctx.addIssue({
          code: "custom",
          path: ["access_starts_at"],
          message: t("organizations.validation.access_start_required"),
        });
      }
      if (
        values.access_starts_at &&
        values.access_ends_at &&
        values.access_ends_at <= values.access_starts_at
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["access_ends_at"],
          message: t("organizations.validation.access_end_after_start"),
        });
      }
    });
}

type AccessValues = z.infer<ReturnType<typeof createAccessSchema>>;

/** "Durum": active / suspended and the access window. */
function StatusTab({ org }: { org: OrganizationSummary }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.organizations.write);
  const setStatus = useSetOrganizationStatus();
  const setAccess = useSetOrganizationAccess();
  const [confirm, setConfirm] = useState(false);
  const schema = useMemo(() => createAccessSchema(t), [t]);
  const suspending = org.status === "active";

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <EntitySectionCard title={t("organizations.status_tab.status_title")}>
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-2">
            <OrganizationStatusChip status={org.status} />
            <OrganizationAccessChip state={org.access_state} />
          </div>
          <p className="text-muted-foreground text-sm">
            {org.status === "active"
              ? t("organizations.status_tab.active_hint")
              : t("organizations.status_tab.suspended_hint")}
          </p>
          {canWrite ? (
            <Button
              type="button"
              variant={suspending ? "destructive" : "default"}
              onClick={() => setConfirm(true)}
            >
              {suspending
                ? t("organizations.actions.suspend")
                : t("organizations.actions.activate")}
            </Button>
          ) : null}
        </div>
      </EntitySectionCard>

      <EntitySectionCard title={t("organizations.status_tab.access_title")}>
        <AppForm<AccessValues>
          key={`${org.access_starts_at}|${org.access_ends_at ?? ""}`}
          className="space-y-4"
          schema={schema}
          defaultValues={{
            access_starts_at: isoToLocalInput(org.access_starts_at),
            access_ends_at: isoToLocalInput(org.access_ends_at),
          }}
          onSubmit={async (values) => {
            await setAccess.mutateAsync({
              uuid: org.uuid,
              body: {
                access_starts_at: localInputToISO(values.access_starts_at),
                access_ends_at: values.access_ends_at
                  ? (localInputToISO(values.access_ends_at) ?? null)
                  : null,
              },
            });
          }}
        >
          {(form) => (
            <>
              <p className="text-muted-foreground text-sm">
                {org.access_ends_at
                  ? t("organizations.status_tab.access_until", {
                      date: datetime(org.access_ends_at, undefined, locale),
                    })
                  : t("organizations.status_tab.access_unlimited")}
              </p>
              <div className="grid gap-4 sm:grid-cols-2">
                <AppDatePicker
                  name="access_starts_at"
                  withTime
                  label={t("organizations.fields.access_starts_at")}
                  disabled={!canWrite}
                />
                <AppDatePicker
                  name="access_ends_at"
                  withTime
                  label={t("organizations.fields.access_ends_at")}
                  description={t("organizations.status_tab.access_ends_hint")}
                  disabled={!canWrite}
                />
              </div>
              {canWrite ? (
                <FormActions>
                  <Button
                    type="button"
                    variant="outline"
                    disabled={form.formState.isSubmitting}
                    onClick={() =>
                      form.setValue("access_ends_at", "", { shouldDirty: true })
                    }
                  >
                    {t("organizations.status_tab.clear_end")}
                  </Button>
                  <Button
                    type="submit"
                    disabled={
                      !form.formState.isDirty || form.formState.isSubmitting
                    }
                  >
                    {t("organizations.actions.save")}
                  </Button>
                </FormActions>
              ) : null}
            </>
          )}
        </AppForm>
      </EntitySectionCard>

      <ConfirmDialog
        open={confirm}
        title={
          suspending
            ? t("organizations.status_confirm.suspend_title", {
                name: org.name,
              })
            : t("organizations.status_confirm.activate_title", {
                name: org.name,
              })
        }
        description={
          suspending
            ? t("organizations.status_confirm.suspend_description")
            : t("organizations.status_confirm.activate_description")
        }
        confirmLabel={
          suspending
            ? t("organizations.actions.suspend")
            : t("organizations.actions.activate")
        }
        variant={suspending ? "destructive" : "default"}
        isPending={setStatus.isPending}
        onCancel={() => setConfirm(false)}
        onConfirm={() =>
          setStatus.mutate(
            { uuid: org.uuid, status: suspending ? "suspended" : "active" },
            { onSettled: () => setConfirm(false) },
          )
        }
      />
    </div>
  );
}

function PlaceholderTab({ title, body }: { title: string; body: string }) {
  return (
    <EntitySectionCard title={title}>
      <EmptyState className="py-10" title={body} />
    </EntitySectionCard>
  );
}

function actorName(entry: OrganizationActivityEntry) {
  if (!entry.actor) return null;
  return (
    `${entry.actor.name} ${entry.actor.surname}`.trim() || entry.actor.email
  );
}

/** "Aktivite": platform operations plus everything recorded in the organization. */
function ActivityTab({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const [limit, setLimit] = useState(20);
  const query = useOrganizationActivity(uuid, limit);
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;
  const describe = (entry: OrganizationActivityEntry) => {
    const key = `activity.actions.${entry.action}`;
    const label = t(key);
    return label === key ? entry.action : label;
  };
  return (
    <EntitySectionCard
      title={t("organizations.tabs.activity")}
      badge={query.data ? total : undefined}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {query.data && items.length === 0 ? (
        <EmptyState
          className="py-8"
          title={t("organizations.activity.empty")}
        />
      ) : null}
      {items.length > 0 ? (
        <div className="space-y-4">
          <Timeline
            items={items.map((entry) => ({
              id: entry.uuid,
              title: describe(entry),
              description:
                actorName(entry) ?? t("organizations.activity.system_actor"),
              meta: (
                <time
                  dateTime={entry.created_at}
                  title={datetime(entry.created_at, undefined, locale)}
                >
                  {relativeDatetime(entry.created_at, locale)}
                </time>
              ),
            }))}
          />
          {items.length < total ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setLimit((value) => value + 20)}
            >
              {t("organizations.activity.more")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

/**
 * Platform organization page (TEC-670): header with status and access
 * state, tabs Genel / Üyeler / Durum / Paket / Alan adları / Bağlantılar /
 * Aktivite (`?tab=`).
 */
export function OrganizationDetailPage({
  uuid,
  initialTab,
}: {
  uuid: string;
  initialTab?: string;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const pathname = usePathname();
  const query = useOrganization(uuid);
  const remove = useDeleteOrganization();
  const [tab, setTab] = useState<Tab>(() => resolveTab(initialTab));
  const [confirmDelete, setConfirmDelete] = useState(false);
  const openTab = (next: string) => {
    const resolved = resolveTab(next);
    setTab(resolved);
    router.replace(`${pathname}?tab=${resolved}`, { scroll: false });
  };
  const org = query.data;
  const title = org?.name ?? t("organizations.detail_title");
  const canWrite = can(permissions.organizations.write);

  return (
    <EntityPage
      title={title}
      description={t("organizations.detail_description")}
      permission={permissions.organizations.read}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        {
          label: t("organizations.title"),
          href: routes.platform.organizations.root,
        },
        { label: title },
      ]}
      actions={
        org && canWrite ? (
          <EntityActions>
            <Button asChild size="sm">
              <Link href={routes.platform.organizations.edit(uuid)}>
                <Pencil className="size-4" />
                {t("organizations.actions.edit")}
              </Link>
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="text-destructive"
              onClick={() => setConfirmDelete(true)}
            >
              <Trash2 className="size-4" />
              {t("organizations.actions.delete")}
            </Button>
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("organizations.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {org ? (
        <div className="space-y-6">
          <EntityHeader
            leading={
              org.logo_url ? (
                <OrganizationAvatar
                  logoUrl={org.logo_url}
                  className="size-14 rounded-xl"
                />
              ) : (
                <span className="bg-muted flex size-14 items-center justify-center rounded-xl border">
                  <Building2 className="text-muted-foreground size-6" />
                </span>
              )
            }
            title={org.name}
            subtitle={<code className="text-xs">{org.slug}</code>}
            badges={
              <>
                <OrganizationStatusChip status={org.status} />
                <OrganizationAccessChip state={org.access_state} />
              </>
            }
          />
          <Tabs value={tab} onValueChange={openTab} className="space-y-4">
            <div className="-mx-1 overflow-x-auto px-1 pb-1">
              <TabsList className="h-auto w-max">
                {TABS.map((value) => (
                  <TabsTrigger key={value} value={value}>
                    {t(TAB_LABEL_KEYS[value])}
                  </TabsTrigger>
                ))}
              </TabsList>
            </div>
            <TabsContent value="general">
              <GeneralTab org={org} />
            </TabsContent>
            <TabsContent value="members">
              <OrganizationMembersTab organizationUuid={uuid} />
            </TabsContent>
            <TabsContent value="status">
              <StatusTab org={org} />
            </TabsContent>
            <TabsContent value="plan">
              <PlatformEntitlementsTab uuid={org.uuid} />
            </TabsContent>
            <TabsContent value="domains">
              <PlaceholderTab
                title={t("organizations.tabs.domains")}
                body={t("organizations.placeholders.domains")}
              />
            </TabsContent>
            <TabsContent value="integrations">
              <PlaceholderTab
                title={t("organizations.tabs.integrations")}
                body={t("organizations.placeholders.integrations")}
              />
            </TabsContent>
            <TabsContent value="activity">
              <ActivityTab uuid={uuid} />
            </TabsContent>
          </Tabs>
        </div>
      ) : null}

      <EntityDeleteDialog
        open={confirmDelete}
        entityLabel={org?.name ?? ""}
        isPending={remove.isPending}
        onCancel={() => setConfirmDelete(false)}
        onConfirm={() =>
          remove.mutate(uuid, {
            onSuccess: () => router.push(routes.platform.organizations.root),
            onSettled: () => setConfirmDelete(false),
          })
        }
      />
    </EntityPage>
  );
}
