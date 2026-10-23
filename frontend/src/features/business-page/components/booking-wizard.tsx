"use client";

import { ArrowLeft, Loader2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useRef, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import { CheckoutTimer } from "@/features/business-page/components/checkout-timer";
import { BranchStep } from "@/features/business-page/components/steps/branch-step";
import {
  CONTACT_FORM_ID,
  ContactStep,
} from "@/features/business-page/components/steps/contact-step";
import { DatetimeStep } from "@/features/business-page/components/steps/datetime-step";
import { ServiceStep } from "@/features/business-page/components/steps/service-step";
import { StaffStep } from "@/features/business-page/components/steps/staff-step";
import { SummaryStep } from "@/features/business-page/components/steps/summary-step";
import {
  usePublicAvailability,
  usePublicServices,
  usePublicStaff,
} from "@/features/business-page/hooks/use-public-booking";
import {
  rememberAccessToken,
  statusHref,
} from "@/features/business-page/lib/access";
import {
  isSlotTaken,
  reportBookingError,
} from "@/features/business-page/lib/errors";
import {
  WINDOW_DAYS,
  checkoutPrices,
  flowHasService,
  flowHasStaff,
  paymentChoices,
  serviceStepKey,
  staffCandidates,
  sumAmounts,
  toLines,
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
  WizardStep,
} from "@/features/business-page/types";
import { addDays } from "@/features/reservations/lib/calendar-time";
import { dateKeyInZone, uuid } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Booking wizard of the business page: branch → service(s) → person →
 * day and time → contact → summary. The steps follow the sector's flow
 * (`business.flow`); every rule (availability, prices, approval, payment)
 * is the API's. Picking a time holds it (`checkouts`) for the minutes of
 * `booking.checkout_minutes`; going back frees it.
 */
export function BookingWizard({
  slug,
  business,
  branches,
}: {
  slug: string;
  business: PublicBusiness;
  branches: PublicBranch[];
}) {
  const { t } = useLocale();
  const router = useRouter();
  const today = useMemo(
    () => dateKeyInZone(new Date(), business.timezone),
    [business.timezone],
  );

  const [step, setStep] = useState<WizardStep | null>(null);
  const [branchUuid, setBranchUuid] = useState(
    branches.length === 1 ? branches[0]!.uuid : "",
  );
  const [pickedServices, setPickedServices] = useState<string[]>([]);
  const [staffBy, setStaffBy] = useState<Record<string, string | null>>({});
  const [windowStart, setWindowStart] = useState(today);
  const [date, setDate] = useState("");
  const [slot, setSlot] = useState<PublicSlot | null>(null);
  const [checkout, setCheckout] = useState<PublicCheckout | null>(null);
  const [contact, setContact] = useState<ContactValues | null>(null);
  const [payment, setPayment] = useState<PaymentMethod | undefined>();
  const [busy, setBusy] = useState(false);
  const bookingKey = useRef("");

  const servicesQuery = usePublicServices(slug, branchUuid);
  const staffQuery = usePublicStaff(slug, branchUuid);
  const services = useMemo(
    () => servicesQuery.data ?? [],
    [servicesQuery.data],
  );
  const staff = useMemo(() => staffQuery.data ?? [], [staffQuery.data]);

  // A flow without a service step (restaurant) books the branch's only
  // service by itself and asks only when there are several offers.
  const autoService = !flowHasService(business.flow) && services.length === 1;
  const serviceUuids = autoService ? [services[0]!.uuid] : pickedServices;
  const selected = serviceUuids
    .map((id) => services.find((s) => s.uuid === id))
    .filter((s): s is PublicService => Boolean(s));
  const staffUuids = staff.map((s) => s.uuid);
  const staffChoice =
    flowHasStaff(business.flow) &&
    selected.some((s) => staffCandidates(s, staffUuids).length > 0);
  const steps = wizardSteps({
    branchCount: branches.length,
    serviceChoice: !autoService,
    staffChoice,
  });
  const current: WizardStep = step && steps.includes(step) ? step : steps[0]!;
  const index = steps.indexOf(current);

  const lines = toLines(serviceUuids, staffBy);
  const windowEnd = addDays(windowStart, WINDOW_DAYS - 1);
  const availabilityBody: PublicAvailabilityRequest | null =
    current === "datetime" && branchUuid && lines.length > 0
      ? {
          branch_uuid: branchUuid,
          lines,
          from: windowStart,
          to: windowEnd,
          step_minutes: 15,
          channel: "web",
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

  const total = checkout ? sumAmounts(checkoutPrices(checkout, services)) : 0;
  const payments = paymentChoices(business, selected, total);
  const activePayment =
    payment && payments.includes(payment) ? payment : payments[0];

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
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

  const back = () => {
    if (index <= 0) return;
    const previous = steps[index - 1]!;
    if (current === "contact") releaseCheckout();
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
          channel: "web",
        },
        uuid(),
      );
      setCheckout(held);
      bookingKey.current = uuid();
      goTo("contact");
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
      const reservation = await publicBookingService.book(
        slug,
        toBookingRequest(checkout.token, contact, activePayment),
        bookingKey.current,
      );
      const token = reservation.access_token ?? "";
      rememberAccessToken(reservation.uuid, token);
      router.push(statusHref(slug, reservation.uuid, token));
    } catch (error) {
      reportBookingError(error, t);
      if (isSlotTaken(error)) {
        setCheckout(null);
        resetTime();
        goTo("datetime");
      }
      setBusy(false);
    }
  };

  const canContinue =
    current === "branch"
      ? Boolean(branchUuid)
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

  const catalogLoading =
    Boolean(branchUuid) && (servicesQuery.isLoading || staffQuery.isLoading);

  let body: React.ReactNode;
  if (current === "branch") {
    body = (
      <BranchStep
        branches={branches}
        value={branchUuid}
        onChange={(value) => {
          setBranchUuid(value);
          setPickedServices([]);
          setStaffBy({});
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
        value={staffBy}
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
      />
    );
  } else if (current === "contact") {
    body = (
      <ContactStep
        value={contact}
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
        contact={contact}
        payments={payments}
        payment={activePayment}
        onPaymentChange={setPayment}
      />
    );
  } else {
    body = null;
  }

  const held = current === "contact" || current === "summary";

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
