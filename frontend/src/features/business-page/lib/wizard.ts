import type {
  PaymentMethod,
  PublicBranch,
  PublicBusiness,
  PublicCheckout,
  PublicLineInput,
  PublicService,
  WizardStep,
} from "@/features/business-page/types";

/** Sector flow steps that pick the service lines (`sectors.steps.*`). */
const SERVICE_STEPS = ["service", "appointment_type", "session_type", "class"];

/** Sector flow steps that let the customer pick who serves them. */
const STAFF_STEPS = ["staff", "class"];

/** Most service lines one booking may carry (API limit). */
export const MAX_LINES = 10;

/** Days the date strip asks the slot engine for at once (API max 31). */
export const WINDOW_DAYS = 14;

/**
 * Label key of the service step: the sector's own word (randevu türü, ders
 * …). A flow without one (restaurant) still books a service, because a
 * booking line is a service in the API.
 */
export function serviceStepKey(flow: string[]) {
  const code = flow.find((step) => SERVICE_STEPS.includes(step)) ?? "service";
  return `sectors.steps.${code}`;
}

/** Whether the sector's flow has its own service step. */
export function flowHasService(flow: string[]) {
  return flow.some((step) => SERVICE_STEPS.includes(step));
}

/** Whether the sector's flow offers a choice of person. */
export function flowHasStaff(flow: string[]) {
  return flow.some((step) => STAFF_STEPS.includes(step));
}

/** "4 kişi · Teras" for a party booking; empty without a party. */
export function seatingLabel(
  t: (key: string, params?: Record<string, string | number>) => string,
  partySize: number | null | undefined,
  zone: string | null | undefined,
) {
  if (!partySize) return "";
  return [t("business_page.party.count", { count: partySize }), zone]
    .filter(Boolean)
    .join(" · ");
}

/** Fewest and most people of a party (API limits). */
export const MIN_PARTY = 1;
export const MAX_PARTY = 1000;

/**
 * Whether the sector books by party (restaurant). The public profile has
 * no capacity block; the sector flow carries a `party_size` step exactly
 * when the sector's capacity books by party.
 */
export function flowHasParty(flow: string[]) {
  return flow.includes("party_size");
}

/** Whether the customer picks an area: the branch has more than one. */
export function hasAreaChoice(branch: PublicBranch | undefined) {
  return (branch?.zones.length ?? 0) > 1;
}

/** A typed party size inside the API limits, else null. */
export function validParty(value: number) {
  return Number.isInteger(value) && value >= MIN_PARTY && value <= MAX_PARTY
    ? value
    : null;
}

/**
 * Steps of the wizard.
 * - The branch step is skipped with one branch.
 * - Party size follows the branch when the sector books by party; the area
 *   comes right after it (only with more than one area), because the area
 *   limits the free times the day step shows.
 * - The service step is skipped when the flow has none (restaurant) and
 *   the branch offers exactly one service, which is then booked.
 * - The staff step shows when the sector offers it and a picked service
 *   has people to choose from.
 * Vehicle and product steps have no public API yet and are not shown.
 */
export function wizardSteps({
  branchCount,
  partySize,
  areaChoice,
  serviceChoice,
  staffChoice,
}: {
  branchCount: number;
  partySize: boolean;
  areaChoice: boolean;
  serviceChoice: boolean;
  staffChoice: boolean;
}): WizardStep[] {
  const steps: WizardStep[] = [];
  if (branchCount > 1) steps.push("branch");
  if (partySize) steps.push("party_size");
  if (partySize && areaChoice) steps.push("area");
  if (serviceChoice) steps.push("service");
  if (staffChoice) steps.push("staff");
  steps.push("datetime", "contact", "summary");
  return steps;
}

/** People of a service the customer may pick (shown online, in the branch). */
export function staffCandidates(service: PublicService, staffUuids: string[]) {
  if (service.staff_mode === "none") return [];
  return service.staff.filter((s) => staffUuids.includes(s.staff_uuid));
}

/** Price and duration of a line: the person's own, else the branch's. */
export function linePrice(service: PublicService, staffUuid: string | null) {
  const own = staffUuid
    ? service.staff.find((s) => s.staff_uuid === staffUuid)
    : undefined;
  return {
    price: own?.price ?? service.price,
    duration: own?.duration_minutes ?? service.duration_minutes,
  };
}

/** Request lines in the picked order; null staff = anyone free. */
export function toLines(
  serviceUuids: string[],
  staffByService: Record<string, string | null>,
): PublicLineInput[] {
  return serviceUuids.map((uuid) => ({
    service_uuid: uuid,
    staff_uuid: staffByService[uuid] ?? null,
  }));
}

/**
 * Payment methods the customer may pick: the business offers them and every
 * picked service allows them. Online payment sends the customer to the
 * provider's page (TEC-912), so it is offered next to on site only when the
 * business can take it (`booking.online_payment_ready`); without a working
 * provider it stays only when it is the one way (the business collects it
 * later). A free booking (total 0, e.g. a table) cannot be paid online.
 */
export function paymentChoices(
  business: PublicBusiness,
  services: PublicService[],
  total: number,
): PaymentMethod[] {
  const allowed = (["on_site", "online"] as const).filter(
    (method) =>
      business.booking.payment_methods.includes(method) &&
      (method !== "online" || total > 0) &&
      services.every(
        (service) =>
          service.payment_mode === "both" || service.payment_mode === method,
      ),
  );
  if (business.booking.online_payment_ready) return allowed;
  return allowed.includes("on_site") ? ["on_site"] : allowed;
}

/** Whether the web page takes bookings (else the reason keys). */
export function bookingOpen(business: PublicBusiness) {
  return business.booking.ready && business.booking.channels.web === true;
}

/** Sum of "450.00" amounts as a number. */
export function sumAmounts(amounts: string[]) {
  return amounts.reduce((total, amount) => total + Number(amount), 0);
}

/** Catalog price of each held line (the person's own, else the branch's). */
export function checkoutPrices(
  checkout: PublicCheckout,
  services: PublicService[],
) {
  return checkout.lines.map((line) => {
    const service = services.find((s) => s.uuid === line.service_uuid);
    return service ? linePrice(service, line.staff?.uuid ?? null).price : "0";
  });
}
