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
  ReservationCollectionRequest,
  ReservationCreateRequest,
  ReservationFacets,
  ReservationNotesRequest,
  ReservationPage,
  ReservationPayments,
  ReservationProductOption,
  ReservationProductRequest,
  ReservationRescheduleRequest,
} from "@/features/reservations/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/reservations";

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

  availability(slug: string, body: AvailabilitySearchRequest) {
    return tenantRequest<Availability>(
      slug,
      "POST",
      "/v1/tenant/availability/search",
      { body, silent: true },
    );
  },
};
