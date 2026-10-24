"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { BadgeCheck, History, Pencil, Trash2 } from "lucide-react";
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
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CustomerNotificationHistoryCard } from "@/features/customer-notifications";
import { CustomerActivityCard } from "@/features/customers/components/customer-activity-card";
import {
  useCustomer,
  useDeleteCustomer,
  useRestoreCustomerHistory,
} from "@/features/customers/hooks/use-customers";
import {
  sourceLabel,
  verificationLabel,
} from "@/features/customers/lib/labels";
import { CustomerBalanceCard } from "@/features/sales";
import { CustomerPackagesCard } from "@/features/session-packages";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Show page: contact, notes and tags, consent, verification, activity. The
 * verification card also carries the person's panel history at this
 * business (TEC-991): when a number release hid the earlier visits, the
 * reception reopens them after checking who the person is.
 */
export function CustomerDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useCustomer(uuid);
  const remove = useDeleteCustomer();
  const restore = useRestoreCustomerHistory();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [confirmRestore, setConfirmRestore] = useState(false);
  const customer = query.data;
  const verification = customer?.verification ?? null;
  const title = customer?.full_name ?? t("customers.detail_title");
  const consent = (value: boolean) =>
    value ? t("customers.consent.given") : t("customers.consent.none");

  return (
    <EntityPage
      title={title}
      description={t("customers.detail_description")}
      permission={permissions.tenant.customers.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("customers.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("customers.title"),
          href: routes.tenant.customers.root(slug),
        },
        { label: title },
      ]}
      actions={
        customer ? (
          <PermissionGuard permission={permissions.tenant.customers.write}>
            <EntityActions>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className="size-4" />
                {t("customers.actions.delete")}
              </Button>
              <Button asChild size="sm">
                <Link href={routes.tenant.customers.edit(slug, uuid)}>
                  <Pencil className="size-4" />
                  {t("customers.actions.edit")}
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
          description={t("customers.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {customer ? (
        <div className="space-y-6">
          <EntityHeader
            title={customer.full_name}
            subtitle={[customer.phone_display, customer.email]
              .filter(Boolean)
              .join(" · ")}
            badges={
              <>
                <StatusChip
                  label={
                    customer.verification
                      ? t("customers.verified.yes")
                      : t("customers.verified.no")
                  }
                  tone={customer.verification ? "success" : "default"}
                />
                {customer.tags.map((tag) => (
                  <Badge key={tag} variant="secondary">
                    {tag}
                  </Badge>
                ))}
              </>
            }
          />

          <EntitySectionCard title={t("customers.detail.contact")}>
            <EntityDetail
              sections={[
                {
                  id: "contact",
                  fields: [
                    {
                      key: "phone",
                      label: t("customers.fields.phone"),
                      value: customer.phone_display || "—",
                    },
                    {
                      key: "email",
                      label: t("customers.fields.email"),
                      value: customer.email || "—",
                    },
                    {
                      key: "birth_date",
                      label: t("customers.fields.birth_date"),
                      value: customer.birth_date
                        ? date(customer.birth_date, undefined, locale)
                        : "—",
                    },
                    {
                      key: "source",
                      label: t("customers.fields.source"),
                      value: sourceLabel(t, customer.source),
                    },
                    {
                      key: "created_at",
                      label: t("customers.fields.created_at"),
                      value: datetime(customer.created_at, undefined, locale),
                    },
                    {
                      key: "updated_at",
                      label: t("customers.fields.updated_at"),
                      value: datetime(customer.updated_at, undefined, locale),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <EntitySectionCard
            title={t("customers.detail.verification")}
            badge={
              customer.verification
                ? t("customers.verified.yes")
                : t("customers.verified.no")
            }
          >
            {verification ? (
              <div className="space-y-4">
                <p className="flex items-start gap-2 text-sm">
                  <BadgeCheck className="text-primary mt-0.5 size-4 shrink-0" />
                  {t("customers.verification.linked", {
                    method: verificationLabel(t, verification.source),
                    at: datetime(verification.verified_at, undefined, locale),
                  })}
                </p>
                {verification.history_hidden_from ? (
                  <Alert>
                    <History className="size-4" />
                    <AlertTitle>
                      {t("customers.history.hidden_title")}
                    </AlertTitle>
                    <AlertDescription className="space-y-3">
                      <p>
                        {t("customers.history.hidden", {
                          at: datetime(
                            verification.history_hidden_from,
                            undefined,
                            locale,
                          ),
                        })}
                      </p>
                      {verification.history_requested_at ? (
                        <p className="font-medium">
                          {t("customers.history.requested", {
                            at: datetime(
                              verification.history_requested_at,
                              undefined,
                              locale,
                            ),
                          })}
                        </p>
                      ) : null}
                      <p>{t("customers.history.check_hint")}</p>
                      <PermissionGuard
                        permission={permissions.tenant.customers.write}
                      >
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          onClick={() => setConfirmRestore(true)}
                        >
                          <History className="size-4" />
                          {t("customers.history.restore")}
                        </Button>
                      </PermissionGuard>
                    </AlertDescription>
                  </Alert>
                ) : null}
                {!verification.history_hidden_from &&
                verification.history_restored_at ? (
                  <p className="text-muted-foreground flex items-start gap-2 text-sm">
                    <History className="mt-0.5 size-4 shrink-0" />
                    {verification.history_restored_by
                      ? t("customers.history.restored_by", {
                          by: verification.history_restored_by,
                          at: datetime(
                            verification.history_restored_at,
                            undefined,
                            locale,
                          ),
                        })
                      : t("customers.history.restored", {
                          at: datetime(
                            verification.history_restored_at,
                            undefined,
                            locale,
                          ),
                        })}
                  </p>
                ) : null}
              </div>
            ) : (
              <p className="text-muted-foreground text-sm">
                {t("customers.verification.unlinked")}
              </p>
            )}
          </EntitySectionCard>

          <EntitySectionCard title={t("customers.detail.notes")}>
            {customer.notes ? (
              <p className="text-sm whitespace-pre-wrap">{customer.notes}</p>
            ) : (
              <p className="text-muted-foreground text-sm">
                {t("customers.notes_empty")}
              </p>
            )}
          </EntitySectionCard>

          <EntitySectionCard title={t("customers.detail.consent")}>
            <EntityDetail
              sections={[
                {
                  id: "consent",
                  fields: [
                    {
                      key: "sms_consent",
                      label: t("customers.fields.sms_consent"),
                      value: consent(customer.sms_consent),
                    },
                    {
                      key: "email_consent",
                      label: t("customers.fields.email_consent"),
                      value: consent(customer.email_consent),
                    },
                    {
                      key: "consent_updated_at",
                      label: t("customers.fields.consent_updated_at"),
                      value: customer.consent_updated_at
                        ? datetime(
                            customer.consent_updated_at,
                            undefined,
                            locale,
                          )
                        : "—",
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <CustomerBalanceCard customerUuid={uuid} />

          <CustomerPackagesCard customer={{ uuid, name: customer.full_name }} />

          <CustomerNotificationHistoryCard customerUuid={uuid} />

          <CustomerActivityCard customerUuid={uuid} />

          <ConfirmDialog
            open={confirmRestore}
            title={t("customers.history.restore")}
            description={t("customers.history.restore_description", {
              name: customer.full_name,
            })}
            confirmLabel={t("customers.history.restore_confirm")}
            isPending={restore.isPending}
            onConfirm={() =>
              restore.mutate(uuid, {
                onSettled: () => setConfirmRestore(false),
              })
            }
            onCancel={() => setConfirmRestore(false)}
          />

          <EntityDeleteDialog
            open={confirmDelete}
            entityLabel={customer.full_name}
            isPending={remove.isPending}
            onCancel={() => setConfirmDelete(false)}
            onConfirm={() =>
              remove.mutate(uuid, {
                onSuccess: () =>
                  router.push(routes.tenant.customers.root(slug)),
                onSettled: () => setConfirmDelete(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
