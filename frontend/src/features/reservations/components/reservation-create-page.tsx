"use client";

import {
  CalendarClock,
  Car,
  ClipboardList,
  ListChecks,
  MapPin,
  MessageSquareText,
  Plus,
  Trash2,
  UserRound,
  Users,
} from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useFieldArray, useFormContext, useWatch } from "react-hook-form";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { EntityForm, EntityPage } from "@/components/entity";
import {
  AppCombobox,
  AppForm,
  AppInput,
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
import { CustomFieldInputs } from "@/features/reservations/components/custom-field-inputs";
import { CustomerQuickCreateDialog } from "@/features/reservations/components/customer-quick-create-dialog";
import { SlotPicker } from "@/features/reservations/components/slot-picker";
import {
  MAX_LINES,
  VEHICLE_TYPE_KEYS,
  VEHICLE_TYPES,
} from "@/features/reservations/constants";
import {
  useCreateReservation,
  useReservationForm,
} from "@/features/reservations/hooks/use-reservations";
import { useSlotSearch } from "@/features/reservations/hooks/use-slot-search";
import { useZoneOptions } from "@/features/reservations/hooks/use-zone-options";
import { clock, zoned } from "@/features/reservations/lib/calendar-time";
import {
  customDefaults,
  panelFields,
  toCustomFields,
} from "@/features/reservations/lib/custom-fields";
import { WRITE_ANY } from "@/features/reservations/lib/permissions";
import { staffOptions } from "@/features/reservations/lib/staff-options";
import {
  ANY_STAFF,
  ANY_ZONE,
  createBookingSchema,
  NO_BUNDLE,
  NO_VEHICLE,
  toLineInputs,
  toPartySize,
  toVehicle,
  toZone,
  type BookingFormValues,
} from "@/features/reservations/schemas/reservation-forms";
import type { ReservationFormField } from "@/features/reservations/types";
import { useSector } from "@/features/sectors/hooks/use-sector";
import { useServiceBundlesList } from "@/features/service-bundles/hooks/use-service-bundles";
import { priceLabel } from "@/features/services/lib/format";
import { useServicesList } from "@/features/services/hooks/use-services";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useSelectedBranch } from "@/features/tenant/hooks/use-selected-branch";
import { dateKeyInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

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
  const { sector } = useSector();
  // Restaurants book a table for a party: guests and area come first.
  const party = Boolean(sector?.capacity.party_size);
  // Car washes ask for the vehicle (type and plate).
  const vehicle = Boolean(sector?.flow.includes("vehicle"));
  // The business's booking questions asked in the panel (TEC-905).
  const bookingForm = useReservationForm();
  const fields = useMemo(
    () => panelFields(bookingForm.data?.fields),
    [bookingForm.data],
  );
  const schema = useMemo(
    () => createBookingSchema(t, { party, fields }),
    [t, party, fields],
  );
  const zone = organization?.timezone ?? undefined;

  // From the calendar: ?branch=&date=&at=HH:MM&staff= (free space clicked).
  const search = useSearchParams();
  const preset = useMemo(
    () => ({
      branch: search.get("branch") ?? "",
      date: /^\d{4}-\d{2}-\d{2}$/.test(search.get("date") ?? "")
        ? search.get("date")!
        : "",
      at: /^\d{2}:\d{2}$/.test(search.get("at") ?? "") ? search.get("at")! : "",
      staff: search.get("staff") ?? "",
    }),
    [search],
  );

  const defaultValues = useMemo<BookingFormValues | null>(() => {
    if (!branches.data || bookingForm.isLoading) return null;
    const items = branches.data.items;
    const branch =
      items.find((item) => item.uuid === preset.branch)?.uuid ??
      items.find((item) => item.uuid === selectedBranch)?.uuid ??
      (items.length === 1 ? items[0].uuid : "");
    return {
      branch_uuid: branch,
      customer_uuid: "",
      bundle_uuid: NO_BUNDLE,
      lines: [{ ...EMPTY_LINE, staff_uuid: preset.staff || ANY_STAFF }],
      party_size: "",
      zone: ANY_ZONE,
      vehicle_type: NO_VEHICLE,
      vehicle_plate: "",
      date: preset.date || dateKeyInZone(new Date(), zone),
      starts_at: "",
      customer_note: "",
      internal_note: "",
      custom_fields: customDefaults(fields),
    };
  }, [
    branches.data,
    bookingForm.isLoading,
    fields,
    preset,
    selectedBranch,
    zone,
  ]);

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
      {branches.isLoading || bookingForm.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : null}
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
                // A combined bundle (TEC-895): its services in order.
                bundle_uuid:
                  values.bundle_uuid !== NO_BUNDLE
                    ? values.bundle_uuid
                    : undefined,
                lines: toLineInputs(values.lines),
                party_size: toPartySize(values.party_size),
                zone: toZone(values.zone),
                vehicle: toVehicle(values.vehicle_type, values.vehicle_plate),
                starts_at: values.starts_at,
                customer_note: values.customer_note || undefined,
                internal_note: values.internal_note || undefined,
                custom_fields: toCustomFields(fields, values.custom_fields),
              });
              router.push(
                routes.tenant.reservations.detail(slug, created.uuid),
              );
            }}
          >
            {(form) => (
              <BookingFields
                fields={fields}
                party={party}
                vehicle={vehicle}
                preferredTime={preset.at}
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
  fields,
  party,
  vehicle,
  preferredTime,
  branchOptions,
  isSubmitting,
  onCancel,
}: {
  /** Booking questions asked in the panel (TEC-905). */
  fields: ReservationFormField[];
  /** The sector books by party size (restaurant flow). */
  party: boolean;
  /** The sector asks for the vehicle (car wash flow). */
  vehicle: boolean;
  /** "HH:MM" to pick once its slot is offered (calendar click). */
  preferredTime: string;
  branchOptions: { value: string; label: string }[];
  isSubmitting: boolean;
  onCancel: () => void;
}) {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const { can } = usePermission();
  const form = useFormContext<BookingFormValues>();
  const { control, setValue } = form;
  const lineArray = useFieldArray({ control, name: "lines" });
  const [branchUuid, lines, date, startsAt, partyInput, zoneInput, bundleUuid] =
    useWatch({
      control,
      name: [
        "branch_uuid",
        "lines",
        "date",
        "starts_at",
        "party_size",
        "zone",
        "bundle_uuid",
      ],
    });
  const partySize = toPartySize(partyInput);
  const areaZone = toZone(zoneInput);
  const zoneOptions = useZoneOptions(party);
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

  // Combined bundles offered in the branch (TEC-895).
  const bundles = useServiceBundlesList(
    {
      limit: 100,
      offset: 0,
      sort: "name",
      branch: branchUuid || undefined,
      is_active: "true",
    },
    {
      enabled: Boolean(branchUuid) && can(permissions.tenant.services.read),
    },
  );
  const bookableBundles = useMemo(
    () => (bundles.data?.items ?? []).filter((bundle) => bundle.bookable),
    [bundles.data?.items],
  );
  const bundle = bookableBundles.find((item) => item.uuid === bundleUuid);
  const bundled = bundleUuid !== NO_BUNDLE;

  // A bundle books its services in order; each keeps its own person.
  const lastBundle = useRef(bundleUuid);
  useEffect(() => {
    if (lastBundle.current === bundleUuid) return;
    lastBundle.current = bundleUuid;
    if (bundleUuid === NO_BUNDLE) {
      lineArray.replace([EMPTY_LINE]);
      return;
    }
    if (!bundle) return;
    lineArray.replace(
      bundle.items.map((item) => ({
        service_uuid: item.service_uuid,
        staff_uuid: ANY_STAFF,
      })),
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps -- replace is stable per field array
  }, [bundle, bundleUuid]);

  // A group class (gym) books seats of a session: how many people.
  const group = (services.data?.items ?? []).some(
    (service) =>
      service.group_capacity != null &&
      lines.some((line) => line.service_uuid === service.uuid),
  );

  const { query, ready } = useSlotSearch({
    branchUuid,
    lines,
    date,
    partySize,
    zone: areaZone,
    partyRequired: party,
  });

  // A restaurant usually offers one service (the table); pick it for them.
  const onlyService =
    party && serviceOptions.length === 1 ? serviceOptions[0].value : "";
  useEffect(() => {
    if (!onlyService || lines.length !== 1 || lines[0]?.service_uuid) return;
    setValue("lines.0.service_uuid", onlyService, { shouldValidate: true });
  }, [lines, onlyService, setValue]);

  // The time clicked on the calendar, picked once when it is free.
  const preferred = useRef(preferredTime);
  const zone = query.data?.timezone;
  useEffect(() => {
    if (!preferred.current || !zone || query.isPlaceholderData) return;
    const slot = query.data?.days[0]?.slots.find(
      (s) => clock(zoned(s.starts_at, zone).minutes) === preferred.current,
    );
    if (!slot) return;
    preferred.current = "";
    setValue("starts_at", slot.starts_at, { shouldValidate: true });
  }, [query.data, query.isPlaceholderData, setValue, zone]);

  // Another branch offers other services and people: start the lines over.
  const lastBranch = useRef(branchUuid);
  useEffect(() => {
    if (lastBranch.current === branchUuid) return;
    lastBranch.current = branchUuid;
    setValue("bundle_uuid", NO_BUNDLE);
    lineArray.replace([EMPTY_LINE]);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- replace is stable per field array
  }, [branchUuid]);

  // The picked start belongs to the old inputs; pick again.
  const linesKey = JSON.stringify(toLineInputs(lines));
  const inputsKey = `${branchUuid}|${linesKey}|${date}|${partySize ?? ""}|${areaZone ?? ""}`;
  const lastInputs = useRef(inputsKey);
  useEffect(() => {
    if (lastInputs.current === inputsKey) return;
    lastInputs.current = inputsKey;
    setValue("starts_at", "");
  }, [inputsKey, setValue]);

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
        ...(vehicle
          ? [
              {
                key: "vehicle",
                label: t("reservations.form.section_vehicle"),
                icon: Car,
              },
            ]
          : []),
        {
          key: "time",
          label: t("reservations.form.section_time"),
          icon: CalendarClock,
        },
        ...(fields.length > 0
          ? [
              {
                key: "questions",
                label: t("reservations.form.section_questions"),
                icon: ClipboardList,
              },
            ]
          : []),
        {
          key: "notes",
          label: t("reservations.form.section_notes"),
          icon: MessageSquareText,
        },
      ]),
    [t, vehicle, fields.length],
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
          {bookableBundles.length > 0 ? (
            <AppSelect
              name="bundle_uuid"
              label={t("reservations.fields.bundle")}
              description={
                bundle
                  ? t("reservations.fields.bundle_price", {
                      price: priceLabel(
                        bundle.price,
                        bundle.currency || organization?.currency || "TRY",
                        locale,
                      ),
                    })
                  : t("reservations.fields.bundle_hint")
              }
              options={[
                {
                  value: NO_BUNDLE,
                  label: t("reservations.fields.bundle_none"),
                },
                ...bookableBundles.map((item) => ({
                  value: item.uuid,
                  label: item.name,
                })),
              ]}
            />
          ) : null}
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
                disabled={!branchUuid || bundled}
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
                disabled={lineArray.fields.length === 1 || bundled}
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
            disabled={
              lineArray.fields.length >= MAX_LINES || !branchUuid || bundled
            }
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

      {vehicle ? (
        <FormSection
          id={sections.id("vehicle")}
          title={t("reservations.form.section_vehicle")}
          description={t("reservations.form.section_vehicle_desc")}
          columns={2}
        >
          <AppSelect
            name="vehicle_type"
            label={t("reservations.fields.vehicle_type")}
            options={[
              {
                value: NO_VEHICLE,
                label: t("reservations.fields.vehicle_none"),
              },
              ...VEHICLE_TYPES.map((type) => ({
                value: type,
                label: t(VEHICLE_TYPE_KEYS[type]),
              })),
            ]}
          />
          <AppInput
            name="vehicle_plate"
            label={t("reservations.fields.vehicle_plate")}
            description={t("reservations.fields.vehicle_plate_hint")}
            startIcon={Car}
            autoComplete="off"
            className="uppercase"
          />
        </FormSection>
      ) : null}

      <FormSection
        id={sections.id("time")}
        title={t("reservations.form.section_time")}
        description={t("reservations.form.section_time_desc")}
      >
        {party || group ? (
          <div className="grid gap-4 sm:grid-cols-2">
            <AppInput
              name="party_size"
              label={t("reservations.fields.party_size")}
              description={t(
                party
                  ? "reservations.fields.party_size_hint"
                  : "reservations.fields.seats_hint",
              )}
              startIcon={Users}
              inputMode="numeric"
              autoComplete="off"
            />
            {party && zoneOptions.length > 0 ? (
              <AppSelect
                name="zone"
                label={t("reservations.fields.zone")}
                description={t("reservations.fields.zone_hint")}
                options={[
                  { value: ANY_ZONE, label: t("reservations.fields.zone_any") },
                  ...zoneOptions,
                ]}
              />
            ) : null}
          </div>
        ) : null}
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

      {fields.length > 0 ? (
        <FormSection
          id={sections.id("questions")}
          title={t("reservations.form.section_questions")}
          description={t("reservations.form.section_questions_desc")}
        >
          <CustomFieldInputs fields={fields} />
        </FormSection>
      ) : null}

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
