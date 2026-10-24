import type {
  PaymentMethod,
  PublicBranch,
  PublicBusiness,
  PublicCheckout,
  PublicClassSession,
  PublicBookingProduct,
  PublicLineInput,
  PublicProduct,
  PublicService,
  PublicSlot,
  PublicVehicle,
  WizardStep,
} from "@/features/business-page/types";
import { vehicleLabel } from "@/features/reservations/lib/labels";
import type { CustomerPackage } from "@/features/customer-panel/types";

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

/**
 * "4 kişi · Teras" for a party booking, "2 kişilik yer" for the seats of a
 * group class (a sector that does not book by party); empty without one.
 */
export function seatingLabel(
  t: (key: string, params?: Record<string, string | number>) => string,
  partySize: number | null | undefined,
  zone: string | null | undefined,
  byParty = true,
) {
  if (!partySize) return "";
  if (!byParty) return t("business_page.seats.count", { count: partySize });
  return [t("business_page.party.count", { count: partySize }), zone]
    .filter(Boolean)
    .join(" · ");
}

/** Whether the sector's flow asks for the vehicle (car wash, TEC-971). */
export function flowHasVehicle(flow: string[]) {
  return flow.includes("vehicle");
}

/** Longest licence plate the API stores (letters, digits and spaces). */
export const MAX_PLATE = 16;

/** A plate as the API stores it: upper case with single spaces. */
export function normalizePlate(plate: string) {
  return plate
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .join(" ")
    .toLocaleUpperCase("tr");
}

/** Whether a typed plate is acceptable (empty = not given). */
export function validPlate(plate: string) {
  const value = normalizePlate(plate);
  return value.length <= MAX_PLATE && /^[\p{L}\p{N} ]*$/u.test(value);
}

/** The vehicle for the booking body (null until a type is picked). */
export function toVehicle(
  type: PublicVehicle["type"] | "",
  plate: string,
): PublicVehicle | null {
  return type ? { type, plate: normalizePlate(plate) } : null;
}

/** "SUV · 34 ABC 123"; empty without a vehicle. */
export function vehicleText(
  t: (key: string) => string,
  vehicle: PublicVehicle | null | undefined,
) {
  return vehicleLabel(t, { vehicle });
}

/** Seats of a group class session; null for an individual service. */
export function groupCapacity(service: PublicService | undefined) {
  return service?.group_capacity ?? null;
}

/**
 * The service picks after a toggle. A group class (TEC-904) is booked on
 * its own (API `group_alone`): picking one replaces the other services,
 * picking another service drops a picked group class.
 */
export function toggleService(
  services: PublicService[],
  picked: string[],
  uuid: string,
  checked: boolean,
): string[] {
  if (!checked) return picked.filter((v) => v !== uuid);
  if (picked.includes(uuid)) return picked;
  const isGroup = (id: string) =>
    groupCapacity(services.find((s) => s.uuid === id)) !== null;
  if (isGroup(uuid)) return [uuid];
  const rest = picked.filter((v) => !isGroup(v));
  return rest.length >= MAX_LINES ? rest : [...rest, uuid];
}

/** Local dates "from" … "to" (inclusive) of a timetable window. */
export function windowDays(from: string, to: string) {
  const out: string[] = [];
  for (let day = from; day <= to && out.length < 62;) {
    out.push(day);
    const next = new Date(`${day}T12:00:00Z`);
    next.setUTCDate(next.getUTCDate() + 1);
    day = next.toISOString().slice(0, 10);
  }
  return out;
}

/**
 * A bookable class session as the slot the checkout holds: its start with
 * the trainer (the API assigns the session's own when none is known).
 */
export function sessionSlot(session: PublicClassSession): PublicSlot {
  return {
    starts_at: session.starts_at,
    ends_at: session.ends_at,
    lines: [
      {
        service_uuid: session.service.uuid,
        starts_at: session.starts_at,
        ends_at: session.ends_at,
        staff: session.trainer,
        seats_left: session.seats_left,
      },
    ],
  };
}

/** Seats a customer may take in a session: 1 up to the class's capacity. */
export function validSeats(value: number, capacity: number) {
  return Number.isInteger(value) && value >= 1 && value <= capacity
    ? value
    : null;
}

/** Whether the sector's flow offers products with the visit (TEC-919). */
export function flowHasProducts(flow: string[]) {
  return flow.includes("products");
}

/** Key of a sellable item: product, or product + variant. */
export function productKey(p: {
  product_uuid: string;
  variant_uuid?: string | null;
}) {
  return p.variant_uuid
    ? `${p.product_uuid}:${p.variant_uuid}`
    : p.product_uuid;
}

/** Picked products of the step as request items (in the list's order). */
export function toProducts(
  products: PublicProduct[],
  quantities: Record<string, number>,
): PublicBookingProduct[] {
  return products.flatMap((p) => {
    const quantity = Math.min(quantities[productKey(p)] ?? 0, p.max_quantity);
    return quantity > 0
      ? [
          {
            product_uuid: p.product_uuid,
            variant_uuid: p.variant_uuid,
            quantity,
          },
        ]
      : [];
  });
}

/** Price of each picked product line (unit price × quantity). */
export function productPrices(
  products: PublicProduct[],
  picked: PublicBookingProduct[],
) {
  return picked.map((item) => {
    const p = products.find((x) => productKey(x) === productKey(item));
    return ((p ? Number(p.price) : 0) * item.quantity).toFixed(2);
  });
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
 * - The product step (TEC-919) follows the time, as in the sector flow,
 *   when the sector sells products with a visit and the branch has some
 *   online; it is optional.
 * - The vehicle step (car wash, TEC-971) follows the branch, before the
 *   service, as in the sector flow; the vehicle goes with the booking.
 * - A group class (gym) has no own step for the seats: the day step asks
 *   how many seats to take, because the free seats are per session.
 */
export function wizardSteps({
  branchCount,
  vehicle = false,
  partySize,
  areaChoice,
  serviceChoice,
  staffChoice,
  productChoice = false,
}: {
  branchCount: number;
  vehicle?: boolean;
  partySize: boolean;
  areaChoice: boolean;
  serviceChoice: boolean;
  staffChoice: boolean;
  productChoice?: boolean;
}): WizardStep[] {
  const steps: WizardStep[] = [];
  if (branchCount > 1) steps.push("branch");
  if (vehicle) steps.push("vehicle");
  if (partySize) steps.push("party_size");
  if (partySize && areaChoice) steps.push("area");
  if (serviceChoice) steps.push("service");
  if (staffChoice) steps.push("staff");
  steps.push("datetime");
  if (productChoice) steps.push("products");
  steps.push("contact", "summary");
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

/**
 * Whether the screen takes bookings: the business page on channel `web`,
 * the embedded screen on `widget` (TEC-908).
 */
export function bookingOpen(
  business: PublicBusiness,
  channel: "web" | "widget" = "web",
) {
  return business.booking.ready && business.booking.channels[channel] === true;
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

/**
 * The signed-in person's packages that can pay each line of the checkout
 * (TEC-984): same service, a branch the package is used in, sessions left
 * and the visit day not after the last day. The API checks it again.
 */
export function packageChoices(
  packages: CustomerPackage[],
  checkout: PublicCheckout,
  visitDay: string,
): CustomerPackage[][] {
  return checkout.lines.map((line) =>
    packages.filter(
      (p) =>
        p.state === "active" &&
        p.remaining > 0 &&
        p.service_uuid === line.service_uuid &&
        (p.all_branches ||
          p.branches.some((b) => b.uuid === checkout.branch.uuid)) &&
        (!p.expires_on || p.expires_on >= visitDay),
    ),
  );
}

/** Picked packages as `package_sessions` (a package once per session left). */
export function toPackageSessions(
  picked: Record<number, string>,
  packages: CustomerPackage[],
) {
  const left = new Map(packages.map((p) => [p.uuid, p.remaining]));
  return Object.entries(picked).flatMap(([index, uuid]) => {
    const remaining = left.get(uuid) ?? 0;
    if (remaining <= 0) return [];
    left.set(uuid, remaining - 1);
    return [{ customer_package_uuid: uuid, line_index: Number(index) }];
  });
}
