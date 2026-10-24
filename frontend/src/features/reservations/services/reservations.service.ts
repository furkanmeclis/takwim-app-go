import type { ResourceMeta } from "@/features/io/types";
import type {
  Availability,
  AvailabilitySearchRequest,
  AgendaParams,
  CalendarParams,
  ListReservationsParams,
  Reservation,
  ReservationAction,
  ReservationActionRequest,
  ReservationAgenda,
  ReservationCalendar,
  ListPaymentFollowUpsParams,
  PaymentFollowUpFacets,
  PaymentFollowUpPage,
  RefundResolveRequest,
  RefundResult,
  ReservationCollectionRequest,
  ReservationCreateRequest,
  ReservationFacets,
  ReservationForm,
  ReservationNotesRequest,
  ReservationPage,
  ReservationPayments,
  ReservationPackageOption,
  ReservationPackageSessionRequest,
  ReservationProductOption,
  ReservationProductRequest,
  ReservationRefundRequest,
  ReservationRescheduleRequest,
} from "@/features/reservations/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/reservations";
const FOLLOW_UPS = "/v1/tenant/payments/follow-ups";

/**
 * Reservations of one organization (`X-Organization: {slug}`). Times are
 * planned by the API's slot engine; the browser only shows its answers.
 */
export const reservationsService = {
  list(slug: string, params: ListReservationsParams) {
    return tenantRequest<ReservationPage>(slug, "GET", BASE, { query: params });
  },

  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },

  facets(slug: string) {
    return tenantRequest<ReservationFacets>(slug, "GET", `${BASE}/facets`);
  },

  /** Booking form questions and the hidden flow steps (TEC-905). */
  form(slug: string) {
    return tenantRequest<ReservationForm>(slug, "GET", `${BASE}/form`);
  },

  /** Columns, working hours and visits of a day or a week. */
  calendar(slug: string, params: CalendarParams) {
    return tenantRequest<ReservationCalendar>(slug, "GET", `${BASE}/calendar`, {
      query: params,
      branch: false,
    });
  },

  /** The signed-in member's own shifts, leave and visits. */
  agenda(slug: string, params: AgendaParams) {
    return tenantRequest<ReservationAgenda>(slug, "GET", `${BASE}/agenda`, {
      query: params,
      branch: false,
    });
  },

  get(slug: string, uuid: string) {
    return tenantRequest<Reservation>(slug, "GET", `${BASE}/${uuid}`);
  },

  create(slug: string, body: ReservationCreateRequest) {
    return tenantRequest<Reservation>(slug, "POST", BASE, { body });
  },

  action(
    slug: string,
    uuid: string,
    action: ReservationAction,
    body: ReservationActionRequest,
  ) {
    return tenantRequest<Reservation>(
      slug,
      "POST",
      `${BASE}/${uuid}/${action}`,
      { body, silent: true },
    );
  },

  reschedule(slug: string, uuid: string, body: ReservationRescheduleRequest) {
    return tenantRequest<Reservation>(
      slug,
      "POST",
      `${BASE}/${uuid}/reschedule`,
      { body, silent: true },
    );
  },

  updateNotes(slug: string, uuid: string, body: ReservationNotesRequest) {
    return tenantRequest<Reservation>(slug, "PATCH", `${BASE}/${uuid}`, {
      body,
    });
  },

  /** Expected, paid and remaining money; ledger entries; online attempts. */
  payments(slug: string, uuid: string) {
    return tenantRequest<ReservationPayments>(
      slug,
      "GET",
      `${BASE}/${uuid}/payments`,
    );
  },

  /** On-site payment (cash, card, transfer) into a till or bank account. */
  collect(slug: string, uuid: string, body: ReservationCollectionRequest) {
    return tenantRequest<ReservationPayments>(
      slug,
      "POST",
      `${BASE}/${uuid}/collections`,
      { body },
    );
  },

  /** Stock items of the visit's branch with their available units. */
  productOptions(slug: string, uuid: string, q?: string) {
    return tenantRequest<{ items: ReservationProductOption[] }>(
      slug,
      "GET",
      `${BASE}/${uuid}/product-options`,
      { query: q ? { q } : {} },
    );
  },

  /** Stock items of a branch for the new reservation form (TEC-977). */
  branchProductOptions(slug: string, branchUuid: string, q?: string) {
    return tenantRequest<{ items: ReservationProductOption[] }>(
      slug,
      "GET",
      `${BASE}/product-options`,
      {
        query: q ? { branch_uuid: branchUuid, q } : { branch_uuid: branchUuid },
      },
    );
  },

  /** Adds a product and reserves its units in the branch's stock. */
  addProduct(slug: string, uuid: string, body: ReservationProductRequest) {
    return tenantRequest<Reservation>(
      slug,
      "POST",
      `${BASE}/${uuid}/products`,
      {
        body,
        silent: true,
      },
    );
  },

  /** Takes a held product off the visit; its units are free again. */
  removeProduct(
    slug: string,
    uuid: string,
    productUuid: string,
    body: ReservationActionRequest,
  ) {
    return tenantRequest<Reservation>(
      slug,
      "DELETE",
      `${BASE}/${uuid}/products/${productUuid}`,
      { body, silent: true },
    );
  },

  /** The customer's packages that can give a session to the visit's lines. */
  packageOptions(slug: string, uuid: string) {
    return tenantRequest<{ items: ReservationPackageOption[] }>(
      slug,
      "GET",
      `${BASE}/${uuid}/package-options`,
    );
  },

  /** Holds the next session of a package for a line (TEC-920). */
  applyPackage(
    slug: string,
    uuid: string,
    body: ReservationPackageSessionRequest,
  ) {
    return tenantRequest<Reservation>(
      slug,
      "POST",
      `${BASE}/${uuid}/package-sessions`,
      { body, silent: true },
    );
  },

  /** Gives a held session back to its package. */
  removePackageSession(
    slug: string,
    uuid: string,
    sessionUuid: string,
    body: ReservationActionRequest,
  ) {
    return tenantRequest<Reservation>(
      slug,
      "DELETE",
      `${BASE}/${uuid}/package-sessions/${sessionUuid}`,
      { body, silent: true },
    );
  },

  /**
   * Pays a collection back: through the provider (online payments it can
   * refund) or recorded by hand. `idempotency_key` makes a retry safe.
   */
  refund(slug: string, uuid: string, body: ReservationRefundRequest) {
    return tenantRequest<RefundResult>(
      slug,
      "POST",
      `${BASE}/${uuid}/refunds`,
      { body },
    );
  },

  /** Settles a provider refund whose result is unknown. */
  resolveRefund(slug: string, uuid: string, body: RefundResolveRequest) {
    return tenantRequest<RefundResult>(
      slug,
      "POST",
      `/v1/tenant/refunds/${uuid}/resolve`,
      { body },
    );
  },

  followUps(slug: string, params: ListPaymentFollowUpsParams) {
    return tenantRequest<PaymentFollowUpPage>(slug, "GET", FOLLOW_UPS, {
      query: params,
    });
  },

  followUpsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${FOLLOW_UPS}/meta`);
  },

  followUpFacets(
    slug: string,
    params: Pick<ListPaymentFollowUpsParams, "q" | "branch">,
  ) {
    return tenantRequest<PaymentFollowUpFacets>(
      slug,
      "GET",
      `${FOLLOW_UPS}/facets`,
      { query: params },
    );
  },

  availability(slug: string, body: AvailabilitySearchRequest) {
    return tenantRequest<Availability>(
      slug,
      "POST",
      "/v1/tenant/availability/search",
      { body, silent: true },
    );
  },
};
