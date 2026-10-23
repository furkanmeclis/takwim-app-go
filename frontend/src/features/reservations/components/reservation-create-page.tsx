"use client";

import {
  CalendarClock,
  ListChecks,
  MapPin,
  MessageSquareText,
  Plus,
  Trash2,
  UserRound,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useFieldArray, useFormContext, useWatch } from "react-hook-form";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { EntityForm, EntityPage } from "@/components/entity";
import {
  AppCombobox,
  AppForm,
  AppSelect,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
  type ComboboxOption,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { customersService } from "@/features/customers/services/customers.service";
import type { Customer } from "@/features/customers/types";
import { CustomerQuickCreateDialog } from "@/features/reservations/components/customer-quick-create-dialog";
import { SlotPicker } from "@/features/reservations/components/slot-picker";
import { MAX_LINES } from "@/features/reservations/constants";
import { useCreateReservation } from "@/features/reservations/hooks/use-reservations";
import { useSlotSearch } from "@/features/reservations/hooks/use-slot-search";
import { WRITE_ANY } from "@/features/reservations/lib/permissions";
import { staffOptions } from "@/features/reservations/lib/staff-options";
import {
  ANY_STAFF,
  createBookingSchema,
  toLineInputs,
  type BookingFormValues,
} from "@/features/reservations/schemas/reservation-forms";
import { useServicesList } from "@/features/services/hooks/use-services";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useSelectedBranch } from "@/features/tenant/hooks/use-selected-branch";
import { dateKeyInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const EMPTY_LINE = { service_uuid: "", staff_uuid: ANY_STAFF };

function customerOption(customer: Customer): ComboboxOption {
  return {
    value: customer.uuid,
    label: customer.full_name,
    description: customer.phone_display || customer.email || undefined,
  };
}

/**
 * Panel booking: branch, customer, services (with an optional person each),
 * then a free start from the slot engine. The booking is confirmed at once
 * and paid at the business.
 */
export function ReservationCreatePage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const create = useCreateReservation();
  const branches = useAccessibleBranches();
  const [selectedBranch] = useSelectedBranch(slug);
  const schema = useMemo(() => createBookingSchema(t), [t]);
  const zone = organization?.timezone ?? undefined;

  const defaultValues = useMemo<BookingFormValues | null>(() => {
    if (!branches.data) return null;
    const items = branches.data.items;
    const branch =
      items.find((item) => item.uuid === selectedBranch)?.uuid ??
      (items.length === 1 ? items[0].uuid : "");
    return {
      branch_uuid: branch,
      customer_uuid: "",
      lines: [EMPTY_LINE],
      date: dateKeyInZone(new Date(), zone),
      starts_at: "",
      customer_note: "",
      internal_note: "",
    };
  }, [branches.data, selectedBranch, zone]);

  return (
    <EntityPage
      title={t("reservations.create_title")}
      description={t("reservations.create_description")}
      permission={WRITE_ANY}
      permissionMode="any"
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("reservations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("reservations.title"),
          href: routes.tenant.reservations.root(slug),
        },
        { label: t("reservations.create_title") },
      ]}
    >
      {branches.isLoading ? <Loading label={t("common.loading")} /> : null}
      {branches.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void branches.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {defaultValues ? (
        <EntityForm>
          <AppForm<BookingFormValues>
            schema={schema}
            defaultValues={defaultValues}
            onSubmit={async (values) => {
              const created = await create.mutateAsync({
                branch_uuid: values.branch_uuid,
                customer_uuid: values.customer_uuid,
                lines: toLineInputs(values.lines),
                starts_at: values.starts_at,
                customer_note: values.customer_note || undefined,
                internal_note: values.internal_note || undefined,
              });
              router.push(
                routes.tenant.reservations.detail(slug, created.uuid),
              );
            }}
          >
            {(form) => (
              <BookingFields
                branchOptions={(branches.data?.items ?? []).map((branch) => ({
                  value: branch.uuid,
                  label: branch.name,
                }))}
                isSubmitting={create.isPending || form.formState.isSubmitting}
                onCancel={() =>
                  router.push(routes.tenant.reservations.root(slug))
                }
              />
            )}
          </AppForm>
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}

function BookingFields({
  branchOptions,
  isSubmitting,
  onCancel,
}: {
  branchOptions: { value: string; label: string }[];
  isSubmitting: boolean;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const form = useFormContext<BookingFormValues>();
  const { control, setValue } = form;
  const lineArray = useFieldArray({ control, name: "lines" });
  const [branchUuid, lines, date, startsAt] = useWatch({
    control,
    name: ["branch_uuid", "lines", "date", "starts_at"],
  });
  const [customerDialog, setCustomerDialog] = useState(false);
  const [createdCustomers, setCreatedCustomers] = useState<ComboboxOption[]>(
    [],
  );

  const services = useServicesList({
    limit: 100,
    offset: 0,
    sort: "name",
    branch: branchUuid || undefined,
    is_active: "true",
  });
  const serviceOptions = useMemo<ComboboxOption[]>(
    () =>
      (services.data?.items ?? []).map((service) => ({
        value: service.uuid,
        label: service.name,
        description: t("services.minutes", {
          minutes: String(service.duration_minutes),
        }),
      })),
    [services.data?.items, t],
  );

  const { query, ready } = useSlotSearch({ branchUuid, lines, date });

  // Another branch offers other services and people: start the lines over.
  const lastBranch = useRef(branchUuid);
  useEffect(() => {
    if (lastBranch.current === branchUuid) return;
    lastBranch.current = branchUuid;
    lineArray.replace([EMPTY_LINE]);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- replace is stable per field array
  }, [branchUuid]);

  // The picked start belongs to the old inputs; pick again.
  const linesKey = JSON.stringify(toLineInputs(lines));
  const lastInputs = useRef(`${branchUuid}|${linesKey}|${date}`);
  useEffect(() => {
    const next = `${branchUuid}|${linesKey}|${date}`;
    if (lastInputs.current === next) return;
    lastInputs.current = next;
    setValue("starts_at", "");
  }, [branchUuid, date, linesKey, setValue]);

  const loadCustomers = useCallback(
    async (q: string) => {
      const page = await customersService.list(slug, {
        limit: 20,
        offset: 0,
        q: q || undefined,
      });
      return page.items.map(customerOption);
    },
    [slug],
  );

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "where",
          label: t("reservations.form.section_where"),
          icon: MapPin,
        },
        {
          key: "services",
          label: t("reservations.form.section_services"),
          icon: ListChecks,
        },
        {
          key: "time",
          label: t("reservations.form.section_time"),
          icon: CalendarClock,
        },
        {
          key: "notes",
          label: t("reservations.form.section_notes"),
          icon: MessageSquareText,
        },
      ]),
    [t],
  );

  return (
    <FormLayout navItems={sections.navItems}>
      <FormSection
        id={sections.id("where")}
        title={t("reservations.form.section_where")}
        description={t("reservations.form.section_where_desc")}
        columns={2}
      >
        <AppSelect
          name="branch_uuid"
          label={t("reservations.fields.branch")}
          placeholder={t("reservations.fields.branch_placeholder")}
          options={branchOptions}
        />
        <div className="space-y-2">
          <AppCombobox
            name="customer_uuid"
            label={t("reservations.fields.customer")}
            placeholder={t("reservations.fields.customer_placeholder")}
            searchPlaceholder={t("reservations.fields.customer_search")}
            loadOptions={loadCustomers}
            options={createdCustomers}
          />
          <PermissionGuard permission={permissions.tenant.customers.write}>
            <Button
              type="button"
              variant="link"
              size="sm"
              className="h-auto px-0"
              onClick={() => setCustomerDialog(true)}
            >
              <UserRound className="size-4" />
              {t("reservations.actions.new_customer")}
            </Button>
          </PermissionGuard>
        </div>
      </FormSection>

      <FormSection
        id={sections.id("services")}
        title={t("reservations.form.section_services")}
        description={t("reservations.form.section_services_desc")}
      >
        <div className="space-y-3">
          {lineArray.fields.map((field, index) => (
            <div
              key={field.id}
              className="grid items-end gap-3 rounded-md border p-3 sm:grid-cols-[1fr_1fr_auto]"
            >
              <AppCombobox
                name={`lines.${index}.service_uuid`}
                label={t("reservations.fields.service")}
                placeholder={t("reservations.fields.service_placeholder")}
                options={serviceOptions}
                disabled={!branchUuid}
              />
              <AppSelect
                name={`lines.${index}.staff_uuid`}
                label={t("reservations.fields.staff")}
                options={staffOptions(t, query.data, index)}
                disabled={!lines[index]?.service_uuid}
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={t("reservations.actions.remove_line")}
                disabled={lineArray.fields.length === 1}
                onClick={() => lineArray.remove(index)}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))}
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={lineArray.fields.length >= MAX_LINES || !branchUuid}
            onClick={() => lineArray.append(EMPTY_LINE)}
          >
            <Plus className="size-4" />
            {t("reservations.actions.add_line")}
          </Button>
          {form.formState.errors.lines?.message ? (
            <p className="text-destructive text-sm">
              {form.formState.errors.lines.message}
            </p>
          ) : null}
        </div>
      </FormSection>

      <FormSection
        id={sections.id("time")}
        title={t("reservations.form.section_time")}
        description={t("reservations.form.section_time_desc")}
      >
        <SlotPicker
          query={query}
          ready={ready}
          date={date}
          onDateChange={(next) =>
            setValue("date", next, { shouldValidate: true })
          }
          value={startsAt}
          onChange={(slot) =>
            setValue("starts_at", slot?.starts_at ?? "", {
              shouldValidate: true,
            })
          }
          error={
            form.formState.errors.starts_at?.message ??
            form.formState.errors.date?.message
          }
        />
      </FormSection>

      <FormSection
        id={sections.id("notes")}
        title={t("reservations.form.section_notes")}
        description={t("reservations.form.section_notes_desc")}
      >
        <AppTextarea
          name="customer_note"
          label={t("reservations.fields.customer_note")}
          description={t("reservations.fields.customer_note_hint")}
        />
        <AppTextarea
          name="internal_note"
          label={t("reservations.fields.internal_note")}
          description={t("reservations.fields.internal_note_hint")}
        />
      </FormSection>

      <FormActions>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("form.cancel")}
        </Button>
        <Button type="submit" disabled={isSubmitting}>
          {t("reservations.actions.create")}
        </Button>
      </FormActions>

      <CustomerQuickCreateDialog
        open={customerDialog}
        onOpenChange={setCustomerDialog}
        onCreated={(customer) => {
          setCreatedCustomers((prev) => [customerOption(customer), ...prev]);
          setValue("customer_uuid", customer.uuid, { shouldValidate: true });
        }}
      />
    </FormLayout>
  );
}
