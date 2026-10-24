"use client";

import { ArrowLeft, Loader2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useRef, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import { useBookingSurface } from "@/features/business-page/components/booking-surface";
import { CheckoutTimer } from "@/features/business-page/components/checkout-timer";
import { BranchStep } from "@/features/business-page/components/steps/branch-step";
import {
  CONTACT_FORM_ID,
  ContactStep,
} from "@/features/business-page/components/steps/contact-step";
import { LargePartyHint } from "@/features/business-page/components/large-party-hint";
import { SeatsPicker } from "@/features/business-page/components/seats-picker";
import { AreaStep } from "@/features/business-page/components/steps/area-step";
import { DatetimeStep } from "@/features/business-page/components/steps/datetime-step";
import { PartyStep } from "@/features/business-page/components/steps/party-step";
import { ProductStep } from "@/features/business-page/components/steps/product-step";
import { ServiceStep } from "@/features/business-page/components/steps/service-step";
import { StaffStep } from "@/features/business-page/components/steps/staff-step";
import { SummaryStep } from "@/features/business-page/components/steps/summary-step";
import { VehicleStep } from "@/features/business-page/components/steps/vehicle-step";
import {
  usePublicAvailability,
  usePublicProducts,
  usePublicServices,
  usePublicStaff,
} from "@/features/business-page/hooks/use-public-booking";
import {
  rememberAccessToken,
  statusHref,
} from "@/features/business-page/lib/access";
import {
  isAnswerError,
  isDayFull,
  isOutOfStock,
  isSlotTaken,
  isVehicleError,
  reportBookingError,
} from "@/features/business-page/lib/errors";
import type { Preselect } from "@/features/business-page/lib/embed";
import {
  WINDOW_DAYS,
  checkoutPrices,
  flowHasParty,
  flowHasProducts,
  flowHasService,
  flowHasStaff,
  flowHasVehicle,
  groupCapacity,
  hasAreaChoice,
  packageChoices,
  paymentChoices,
  productPrices,
  serviceStepKey,
  staffCandidates,
  sumAmounts,
  toLines,
  toPackageSessions,
  toProducts,
  toVehicle,
  validParty,
  validPlate,
  validSeats,
  wizardSteps,
} from "@/features/business-page/lib/wizard";
import { toBookingRequest } from "@/features/business-page/schemas/contact-form";
import { publicBookingService } from "@/features/business-page/services/public-booking.service";
import type {
  ContactValues,
  PaymentMethod,
  PublicAvailabilityRequest,
  PublicBranch,
  PublicBusiness,
  PublicCheckout,
  PublicService,
  PublicSlot,
  PublicVehicle,
  WizardStep,
} from "@/features/business-page/types";
import {
  useCustomerPackages,
  useCustomerProfile,
} from "@/features/customer-panel/hooks/use-customer-panel";
import { customerPanelService } from "@/features/customer-panel/services/customer-panel.service";
import { addDays } from "@/features/reservations/lib/calendar-time";
import { dateKeyInZone, uuid } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Booking wizard of the business page: branch → vehicle (car wash) →
 * service(s) → person → day and time (with the seats of a group class) →
 * products (optional) → contact → summary. The steps follow the sector's flow
 * (`business.flow`); every rule (availability, prices, approval, payment)
 * is the API's. Picking a time holds it (`checkouts`) for the minutes of
 * `booking.checkout_minutes`; going back frees it.
 *
 * A pre-selection (TEC-908: `branch`, `service`, `staff`, `date` of the
 * link or the embed code) fills the steps it names when the API lists
 * them, and the wizard opens on the first step still to answer; going back
 * still reaches the pre-filled ones.
 */
export function BookingWizard({
  slug,
  business,
  branches,
  preselect = {},
}: {
  slug: string;
  business: PublicBusiness;
  branches: PublicBranch[];
  preselect?: Preselect;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const surface = useBookingSurface();
  const today = useMemo(
    () => dateKeyInZone(new Date(), business.timezone),
    [business.timezone],
  );
  const presetBranch =
    preselect.branch && branches.some((b) => b.uuid === preselect.branch)
      ? preselect.branch
      : "";
  const presetServices = preselect.service ? [preselect.service] : [];
  const presetDate =
    preselect.date && preselect.date >= today ? preselect.date : "";

  const [step, setStep] = useState<WizardStep | null>(null);
  const [branchUuid, setBranchUuid] = useState(
    branches.length === 1 ? branches[0]!.uuid : presetBranch,
  );
  const [partyText, setPartyText] = useState("2");
  const [vehicleType, setVehicleType] = useState<PublicVehicle["type"] | "">(
    "",
  );
  const [plate, setPlate] = useState("");
  const [seats, setSeats] = useState(1);
  const [zone, setZone] = useState("");
  const [pickedServices, setPickedServices] =
    useState<string[]>(presetServices);
  const [staffBy, setStaffBy] = useState<Record<string, string | null>>({});
  const [windowStart, setWindowStart] = useState(presetDate || today);
  const [date, setDate] = useState(presetDate);
  const [slot, setSlot] = useState<PublicSlot | null>(null);
  const [checkout, setCheckout] = useState<PublicCheckout | null>(null);
  const [productQty, setProductQty] = useState<Record<string, number>>({});
  const [contact, setContact] = useState<ContactValues | null>(null);
  const [payment, setPayment] = useState<PaymentMethod | undefined>();
  // Line index → the customer's package paying it (TEC-984).
  const [packageBy, setPackageBy] = useState<Record<number, string>>({});
  const [busy, setBusy] = useState(false);
  const bookingKey = useRef("");

  const servicesQuery = usePublicServices(slug, branchUuid);
  const staffQuery = usePublicStaff(slug, branchUuid);
  const productsQuery = usePublicProducts(
    slug,
    branchUuid,
    flowHasProducts(business.flow),
  );
  const products = useMemo(
    () => productsQuery.data ?? [],
    [productsQuery.data],
  );
  const services = useMemo(
    () => servicesQuery.data ?? [],
    [servicesQuery.data],
  );
  const staff = useMemo(() => staffQuery.data ?? [], [staffQuery.data]);

  // A flow without a service step (restaurant) books the branch's only
  // service by itself and asks only when there are several offers.
  const autoService = !flowHasService(business.flow) && services.length === 1;
  // Only services the branch offers online (a pre-selected one may not be).
  const serviceUuids = autoService
    ? [services[0]!.uuid]
    : pickedServices.filter((id) => services.some((s) => s.uuid === id));
  const selected = serviceUuids
    .map((id) => services.find((s) => s.uuid === id))
    .filter((s): s is PublicService => Boolean(s));
  const staffUuids = staff.map((s) => s.uuid);
  const staffChoice =
    flowHasStaff(business.flow) &&
    selected.some((s) => staffCandidates(s, staffUuids).length > 0);
  // A pre-selected person serves every picked service they offer, until
  // the customer picks someone else.
  const presetStaff =
    preselect.staff && staffUuids.includes(preselect.staff)
      ? preselect.staff
      : null;
  const staffFor: Record<string, string | null> = {};
  for (const service of selected) {
    staffFor[service.uuid] =
      service.uuid in staffBy
        ? (staffBy[service.uuid] ?? null)
        : presetStaff &&
            staffCandidates(service, staffUuids).some(
              (c) => c.staff_uuid === presetStaff,
            )
          ? presetStaff
          : null;
  }
  const byParty = flowHasParty(business.flow);
  const askVehicle = flowHasVehicle(business.flow);
  // A group class is booked on its own (TEC-904); the booking takes
  // `party_size` seats of the session, 1 up to the class's capacity.
  const classCapacity =
    !byParty && selected.length === 1 ? groupCapacity(selected[0]) : null;
  const activeSeats =
    classCapacity !== null ? (validSeats(seats, classCapacity) ?? 1) : null;
  const branch = branches.find((b) => b.uuid === branchUuid);
  // The area step of the flow (hidden by the business: any area).
  const areaChoice = business.flow.includes("area") && hasAreaChoice(branch);
  const partySize = byParty ? validParty(Number(partyText)) : null;
  // An area that is no longer offered (other branch) means any area.
  const activeZone =
    byParty && areaChoice && branch?.zones.includes(zone) ? zone : "";
  const steps = wizardSteps({
    branchCount: branches.length,
    vehicle: askVehicle,
    partySize: byParty,
    areaChoice,
    serviceChoice: !autoService,
    staffChoice,
    // A product already picked keeps the step while its list reloads.
    productChoice: products.length > 0 || Object.keys(productQty).length > 0,
  });
  const catalogLoading =
    Boolean(branchUuid) && (servicesQuery.isLoading || staffQuery.isLoading);
  // Steps a valid pre-selection already answers; the wizard opens after them
  // (only once the catalog it is checked against has loaded).
  const preset = (s: WizardStep) =>
    (s === "branch" && Boolean(presetBranch) && branchUuid === presetBranch) ||
    (!catalogLoading &&
      s === "service" &&
      Boolean(preselect.service) &&
      serviceUuids.includes(preselect.service!)) ||
    (!catalogLoading &&
      s === "staff" &&
      Boolean(presetStaff) &&
      selected.some((x) => staffFor[x.uuid] === presetStaff));
  const opening = steps.find((s) => !preset(s)) ?? steps[0]!;
  const current: WizardStep = step && steps.includes(step) ? step : opening;
  const index = steps.indexOf(current);

  const lines = toLines(serviceUuids, staffFor);
  // Party and area of a party sector, or the seats of a group class; the
  // checkout keeps them for the booking.
  const seating = byParty
    ? { party_size: partySize, ...(activeZone ? { zone: activeZone } : {}) }
    : activeSeats !== null
      ? { party_size: activeSeats }
      : {};
  const windowEnd = addDays(windowStart, WINDOW_DAYS - 1);
  const availabilityBody: PublicAvailabilityRequest | null =
    current === "datetime" &&
    branchUuid &&
    lines.length > 0 &&
    (!byParty || partySize !== null)
      ? {
          branch_uuid: branchUuid,
          lines,
          from: windowStart,
          to: windowEnd,
          // The business's online interval wins on the API anyway.
          step_minutes: business.booking.slot_step_minutes ?? 15,
          channel: surface.channel,
          ...seating,
        }
      : null;
  const availability = usePublicAvailability(slug, availabilityBody);
  const days = availability.data?.days ?? [];
  // The picked day, else the first day with free starts in the window.
  const activeDate =
    (date && days.some((d) => d.date === date) ? date : "") ||
    days.find((d) => d.slots.length > 0)?.date ||
    days[0]?.date ||
    "";
  const latest = availability.data?.latest_date ?? null;

  // A signed-in customer panel session that this business knows can pay
  // lines with package sessions (TEC-984); the API checks every rule again.
  const profile = useCustomerProfile();
  const linked = Boolean(
    profile.data?.businesses.some((b) => b.uuid === business.uuid),
  );
  const packagesQuery = useCustomerPackages(
    { limit: 50, offset: 0, business: business.uuid, state: "active" },
    { enabled: linked && Boolean(checkout) },
  );
  const myPackages = useMemo(
    () => (linked ? (packagesQuery.data?.items ?? []) : []),
    [linked, packagesQuery.data?.items],
  );
  const choices = checkout
    ? packageChoices(
        myPackages,
        checkout,
        dateKeyInZone(new Date(checkout.starts_at), business.timezone),
      )
    : [];
  const sessions = toPackageSessions(
    Object.fromEntries(
      Object.entries(packageBy).filter(([index, id]) =>
        choices[Number(index)]?.some((p) => p.uuid === id),
      ),
    ),
    myPackages,
  );
  const covered = new Set(sessions.map((s) => s.line_index));

  const pickedProducts = toProducts(products, productQty);
  const productTotal = sumAmounts(productPrices(products, pickedProducts));
  const linePrices = checkout ? checkoutPrices(checkout, services) : [];
  const total =
    sumAmounts(linePrices.filter((_, index) => !covered.has(index))) +
    productTotal;
  const payments = paymentChoices(
    business,
    selected.filter((service) =>
      checkout?.lines.some(
        (line, index) =>
          line.service_uuid === service.uuid && !covered.has(index),
      ),
    ),
    total,
  );
  // A visit the packages cover entirely has nothing to pay: at the venue.
  const activePayment =
    sessions.length > 0 && total === 0
      ? "on_site"
      : payment && payments.includes(payment)
        ? payment
        : payments[0];

  const staffTerm = t(`sectors.terms.${business.terms.staff ?? "staff"}`);
  const stepTitle = (s: WizardStep) =>
    s === "service"
      ? t(serviceStepKey(business.flow))
      : s === "staff"
        ? staffTerm
        : t(`business_page.steps.${s}`);

  const resetTime = () => {
    setDate("");
    setSlot(null);
  };

  const releaseCheckout = () => {
    if (!checkout) return;
    const token = checkout.token;
    setCheckout(null);
    void publicBookingService.releaseCheckout(slug, token).catch(() => {
      // The hold runs out by itself.
    });
  };

  const goTo = (next: WizardStep) => {
    setStep(next);
    surface.scrollTop();
  };

  const back = () => {
    if (index <= 0) return;
    const previous = steps[index - 1]!;
    // The time is held from the step after the day; going back to the day
    // frees it.
    if (previous === "datetime") releaseCheckout();
    goTo(previous);
  };

  const holdSlot = async () => {
    if (!slot) return;
    setBusy(true);
    try {
      const held = await publicBookingService.createCheckout(
        slug,
        {
          branch_uuid: branchUuid,
          lines,
          starts_at: slot.starts_at,
          channel: surface.channel,
          ...seating,
        },
        uuid(),
      );
      setCheckout(held);
      setPackageBy({});
      bookingKey.current = uuid();
      goTo(steps[index + 1]!);
    } catch (error) {
      reportBookingError(error, t);
      if (isSlotTaken(error)) {
        setSlot(null);
        void availability.refetch();
      }
    } finally {
      setBusy(false);
    }
  };

  const book = async () => {
    if (!checkout || !contact) return;
    setBusy(true);
    try {
      const request = toBookingRequest(
        checkout.token,
        contact,
        activePayment,
        pickedProducts,
        business.booking.fields,
        askVehicle ? toVehicle(vehicleType, plate) : null,
      );
      // Package sessions need the signed-in route (the session names the
      // customer); everything else books as before.
      const reservation =
        sessions.length > 0
          ? await customerPanelService.book(
              slug,
              { ...request, package_sessions: sessions },
              bookingKey.current,
            )
          : await publicBookingService.book(slug, request, bookingKey.current);
      const token = reservation.access_token ?? "";
      rememberAccessToken(reservation.uuid, token);
      router.push(
        statusHref(slug, reservation.uuid, token, surface.base, surface.query),
      );
    } catch (error) {
      reportBookingError(error, t);
      if (isSlotTaken(error) || isDayFull(error)) {
        setCheckout(null);
        resetTime();
        goTo("datetime");
      } else if (isAnswerError(error)) {
        goTo("contact");
      } else if (isVehicleError(error)) {
        releaseCheckout();
        goTo("vehicle");
      } else if (isOutOfStock(error)) {
        // Someone took the last units; pick again from what is left.
        void productsQuery.refetch();
        goTo("products");
      } else if (sessions.length > 0) {
        // A package changed meanwhile (used elsewhere, expired): pick again.
        setPackageBy({});
        void packagesQuery.refetch();
      }
      setBusy(false);
    }
  };

  const canContinue =
    current === "branch"
      ? Boolean(branchUuid)
      : current === "vehicle"
        ? Boolean(vehicleType) && validPlate(plate)
        : current === "party_size"
          ? partySize !== null
          : current === "service"
            ? serviceUuids.length > 0
            : current === "datetime"
              ? Boolean(slot)
              : current === "summary"
                ? Boolean(checkout && contact)
                : true;

  const next = () => {
    if (current === "datetime") {
      void holdSlot();
      return;
    }
    if (current === "summary") {
      void book();
      return;
    }
    goTo(steps[index + 1]!);
  };

  let body: React.ReactNode;
  if (current === "branch") {
    body = (
      <BranchStep
        branches={branches}
        value={branchUuid}
        onChange={(value) => {
          setBranchUuid(value);
          setZone("");
          setPickedServices(presetServices);
          setStaffBy({});
          setProductQty({});
          resetTime();
        }}
      />
    );
  } else if (current === "vehicle") {
    body = (
      <VehicleStep
        type={vehicleType}
        plate={plate}
        onTypeChange={setVehicleType}
        onPlateChange={setPlate}
      />
    );
  } else if (current === "party_size") {
    body = (
      <PartyStep
        value={partyText}
        onChange={(value) => {
          setPartyText(value);
          resetTime();
        }}
      />
    );
  } else if (current === "area") {
    body = (
      <AreaStep
        zones={branch?.zones ?? []}
        value={activeZone}
        onChange={(value) => {
          setZone(value);
          resetTime();
        }}
      />
    );
  } else if (catalogLoading) {
    body = (
      <div className="space-y-2">
        {Array.from({ length: 3 }, (_, i) => (
          <Skeleton key={i} className="h-16 w-full" />
        ))}
      </div>
    );
  } else if (servicesQuery.isError) {
    body = (
      <ErrorState
        title={t("business_page.service.load_failed")}
        onRetry={() => void servicesQuery.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  } else if (services.length === 0) {
    body = (
      <EmptyState
        title={t("business_page.service.empty_title")}
        description={t("business_page.service.empty_description")}
      />
    );
  } else if (current === "service") {
    body = (
      <ServiceStep
        services={services}
        value={pickedServices}
        currency={business.currency}
        onChange={(value) => {
          setPickedServices(value);
          resetTime();
        }}
      />
    );
  } else if (current === "staff") {
    body = (
      <StaffStep
        services={selected}
        staff={staff}
        value={staffFor}
        staffTerm={staffTerm}
        currency={business.currency}
        onChange={(service, person) => {
          setStaffBy((prev) => ({ ...prev, [service]: person }));
          resetTime();
        }}
      />
    );
  } else if (current === "datetime") {
    body = (
      <DatetimeStep
        query={availability}
        date={activeDate}
        onDateChange={(value) => {
          setDate(value);
          setSlot(null);
        }}
        slot={slot}
        onSlotChange={setSlot}
        canGoBack={windowStart > today}
        canGoForward={!latest || windowEnd < latest}
        onPrevWindow={() => {
          const prev = addDays(windowStart, -WINDOW_DAYS);
          setWindowStart(prev < today ? today : prev);
          resetTime();
        }}
        onNextWindow={() => {
          setWindowStart(addDays(windowStart, WINDOW_DAYS));
          resetTime();
        }}
        header={
          classCapacity !== null && activeSeats !== null ? (
            <SeatsPicker
              value={activeSeats}
              capacity={classCapacity}
              onChange={(value) => {
                setSeats(value);
                setSlot(null);
              }}
            />
          ) : undefined
        }
        noSlotsHint={
          byParty && partySize !== null ? (
            <LargePartyHint
              partySize={partySize}
              phone={branch?.phone || business.contact.phone}
            />
          ) : undefined
        }
      />
    );
  } else if (current === "products") {
    body = (
      <ProductStep
        products={products}
        value={productQty}
        currency={business.currency}
        onChange={setProductQty}
      />
    );
  } else if (current === "contact") {
    body = (
      <ContactStep
        value={contact}
        fields={business.booking.fields}
        onSubmit={(values) => {
          setContact(values);
          goTo("summary");
        }}
      />
    );
  } else if (checkout && contact) {
    body = (
      <SummaryStep
        business={business}
        checkout={checkout}
        services={services}
        products={products}
        pickedProducts={pickedProducts}
        contact={contact}
        payments={sessions.length > 0 && total === 0 ? [] : payments}
        payment={activePayment}
        onPaymentChange={setPayment}
        vehicle={askVehicle ? toVehicle(vehicleType, plate) : null}
        packageChoices={choices}
        packageBy={packageBy}
        covered={covered}
        onPackageChange={(index, id) =>
          setPackageBy((prev) => {
            const nextPicks = { ...prev };
            if (id) nextPicks[index] = id;
            else delete nextPicks[index];
            return nextPicks;
          })
        }
      />
    );
  } else {
    body = null;
  }

  const held = index > steps.indexOf("datetime");

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <p className="text-muted-foreground text-xs">
          {t("business_page.step_of", {
            current: index + 1,
            total: steps.length,
          })}
        </p>
        <h2 className="text-lg font-semibold">{stepTitle(current)}</h2>
        <Progress value={((index + 1) / steps.length) * 100} />
      </div>

      {held && checkout ? (
        <CheckoutTimer
          expiresAt={checkout.expires_at}
          totalMinutes={business.booking.checkout_minutes}
        />
      ) : null}

      {body}

      <div className="bg-background/95 sticky bottom-0 z-10 -mx-4 flex items-center gap-2 border-t px-4 py-3 backdrop-blur sm:mx-0 sm:rounded-lg sm:border">
        {index > 0 ? (
          <Button
            type="button"
            variant="outline"
            onClick={back}
            disabled={busy}
          >
            <ArrowLeft className="size-4" />
            {t("business_page.back")}
          </Button>
        ) : null}
        {current === "contact" ? (
          <Button
            type="submit"
            form={CONTACT_FORM_ID}
            className="ms-auto flex-1 sm:flex-none"
            disabled={busy}
          >
            {t("business_page.continue")}
          </Button>
        ) : (
          <Button
            type="button"
            className="ms-auto flex-1 sm:flex-none"
            onClick={next}
            disabled={!canContinue || busy || catalogLoading}
          >
            {busy ? <Loader2 className="size-4 animate-spin" /> : null}
            {current === "summary"
              ? business.booking.approval_mode === "staff"
                ? t("business_page.submit_request")
                : t("business_page.submit")
              : t("business_page.continue")}
          </Button>
        )}
      </div>
    </div>
  );
}
