"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  History,
  LogOut,
  PhoneForwarded,
  ShieldOff,
  UserX,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityDetail,
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { CustomerDevicesCard } from "@/features/customer-panel/components/customer-devices-card";
import { CustomerPhoneChangeDialog } from "@/features/customer-panel/components/customer-phone-change-dialog";
import { CustomerReassignedDialog } from "@/features/customer-panel/components/customer-reassigned-dialog";
import {
  useCustomerLogout,
  useCustomerProfile,
} from "@/features/customer-panel/hooks/use-customer-panel";
import { customerErrorMessage } from "@/features/customer-panel/lib/errors";
import type { CustomerBusiness } from "@/features/customer-panel/types";
import { date, datetime } from "@/lib/utils/format";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * `/panel/profile`: the verified phone, the businesses that know it, the
 * signed-in devices and access security (TEC-910): a safe number change,
 * the report of a recently reassigned number, and signing out. Signing out
 * everywhere ends every panel session of this phone (other phones and
 * browsers too). A notice explains history closed without the person
 * asking (TEC-972: a reassignment processed by Takwim, a long unproven
 * number).
 */
export function CustomerProfilePage() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const profile = useCustomerProfile().data;
  const logout = useCustomerLogout();
  const [confirmAll, setConfirmAll] = useState(false);
  const [changingPhone, setChangingPhone] = useState(false);
  const [reassigned, setReassigned] = useState(false);

  const columns = useMemo(
    () =>
      [
        createColumn<CustomerBusiness>({
          accessorKey: "name",
          labelKey: "customer_panel.profile.business",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-2 font-medium">
              {row.original.name}
              {row.original.history_from ? (
                <Badge variant="outline">
                  {t("customer_panel.profile.history_from", {
                    at: date(row.original.history_from, undefined, locale),
                  })}
                </Badge>
              ) : null}
              {row.original.history_until ? (
                <Badge variant="outline">
                  {t("customer_panel.profile.history_until", {
                    at: date(row.original.history_until, undefined, locale),
                  })}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<CustomerBusiness>({
          id: "known_as",
          accessorFn: (row) => `${row.first_name} ${row.last_name}`.trim(),
          labelKey: "customer_panel.profile.known_as",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            `${row.original.first_name} ${row.original.last_name}`.trim() ||
            "—",
        }),
        createColumn<CustomerBusiness>({
          accessorKey: "reservation_count",
          labelKey: "customer_panel.profile.visits",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {row.original.reservation_count}
            </span>
          ),
        }),
        createColumn<CustomerBusiness>({
          accessorKey: "linked_at",
          labelKey: "customer_panel.profile.linked_at",
          enableSorting: false,
          cell: ({ row }) => date(row.original.linked_at, undefined, locale),
        }),
      ] as ColumnDef<CustomerBusiness, unknown>[],
    [locale, t],
  );

  const signOut = (everywhere: boolean) =>
    logout.mutate(everywhere, {
      onSuccess: () => {
        appToast.success(
          everywhere
            ? t("customer_panel.profile.logged_out_everywhere")
            : t("customer_panel.profile.logged_out"),
        );
      },
      onError: (error) => appToast.error(customerErrorMessage(t, error)),
      onSettled: () => {
        setConfirmAll(false);
        router.replace(routes.customer.login);
      },
    });

  if (!profile) return null;

  return (
    <EntityPage
      title={t("customer_panel.profile.title")}
      description={t("customer_panel.profile.description")}
    >
      {profile.history_notice ? (
        <Alert>
          <History className="size-4" />
          <AlertTitle>
            {t("customer_panel.profile.history_notice_title")}
          </AlertTitle>
          <AlertDescription>
            {t(
              profile.history_notice.reason === "dormant"
                ? "customer_panel.profile.history_notice_dormant"
                : "customer_panel.profile.history_notice_reassigned",
              { at: date(profile.history_notice.closed_at, undefined, locale) },
            )}
          </AlertDescription>
        </Alert>
      ) : null}

      <EntitySectionCard title={t("customer_panel.profile.account")}>
        <EntityDetail
          sections={[
            {
              id: "account",
              fields: [
                {
                  key: "phone",
                  label: t("customer_panel.profile.phone"),
                  value: profile.phone_display || profile.phone,
                },
                {
                  key: "verified_at",
                  label: t("customer_panel.profile.verified_at"),
                  value: datetime(profile.verified_at, undefined, locale),
                },
                {
                  key: "session_expires_at",
                  label: t("customer_panel.profile.session_expires_at"),
                  value: datetime(
                    profile.session_expires_at,
                    undefined,
                    locale,
                  ),
                },
                {
                  key: "active_sessions",
                  label: t("customer_panel.profile.active_sessions"),
                  value: profile.active_sessions,
                },
              ],
            },
          ]}
        />
      </EntitySectionCard>

      <EntitySectionCard
        title={t("customer_panel.profile.businesses")}
        badge={profile.businesses.length}
      >
        <EntityTable
          columns={columns}
          data={profile.businesses}
          getRowId={(row) => row.uuid}
          emptyTitle={t("customer_panel.profile.no_businesses")}
          emptyDescription={t(
            "customer_panel.profile.no_businesses_description",
          )}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            rowSelection: false,
            columnFilters: false,
            facetedFilters: false,
            globalFilter: false,
            sorting: false,
            pagination: false,
            persistKey: "customer-panel-businesses-v1",
          }}
        />
      </EntitySectionCard>

      <CustomerDevicesCard />

      <EntitySectionCard title={t("customer_panel.profile.access")}>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="flex flex-col items-start gap-2">
            <p className="text-muted-foreground text-sm">
              {t("customer_panel.profile.change_phone_hint")}
            </p>
            <Button variant="outline" onClick={() => setChangingPhone(true)}>
              <PhoneForwarded className="size-4" />
              {t("customer_panel.phone_change.title")}
            </Button>
          </div>
          <div className="flex flex-col items-start gap-2">
            <p className="text-muted-foreground text-sm">
              {t("customer_panel.profile.reassigned_hint")}
            </p>
            <Button variant="outline" onClick={() => setReassigned(true)}>
              <UserX className="size-4" />
              {t("customer_panel.reassigned.title")}
            </Button>
          </div>
        </div>
      </EntitySectionCard>

      <EntitySectionCard title={t("customer_panel.profile.sessions")}>
        <div className="flex flex-col gap-2 sm:flex-row">
          <Button
            variant="outline"
            disabled={logout.isPending}
            onClick={() => signOut(false)}
          >
            <LogOut className="size-4" />
            {t("customer_panel.profile.logout")}
          </Button>
          <Button
            variant="destructive"
            disabled={logout.isPending}
            onClick={() => setConfirmAll(true)}
          >
            <ShieldOff className="size-4" />
            {t("customer_panel.profile.logout_everywhere")}
          </Button>
        </div>
      </EntitySectionCard>

      <CustomerPhoneChangeDialog
        open={changingPhone}
        onOpenChange={setChangingPhone}
      />
      <CustomerReassignedDialog
        open={reassigned}
        onOpenChange={setReassigned}
        businesses={profile.businesses}
      />
      <ConfirmDialog
        open={confirmAll}
        title={t("customer_panel.profile.logout_everywhere")}
        description={t("customer_panel.profile.logout_everywhere_description")}
        confirmLabel={t("customer_panel.profile.logout_everywhere")}
        variant="destructive"
        isPending={logout.isPending}
        onConfirm={() => signOut(true)}
        onCancel={() => setConfirmAll(false)}
      />
    </EntityPage>
  );
}
