import type { ResourceMeta } from "@/features/io/types";
import type {
  Availability,
  AvailabilitySearchRequest,
  CalendarParams,
  ListReservationsParams,
  Reservation,
  ReservationAction,
  ReservationActionRequest,
  ReservationCalendar,
  ReservationCreateRequest,
  ReservationFacets,
  ReservationNotesRequest,
  ReservationPage,
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
      { body },
    );
  },

  reschedule(slug: string, uuid: string, body: ReservationRescheduleRequest) {
    return tenantRequest<Reservation>(
      slug,
      "POST",
      `${BASE}/${uuid}/reschedule`,
      { body },
    );
  },

  updateNotes(slug: string, uuid: string, body: ReservationNotesRequest) {
    return tenantRequest<Reservation>(slug, "PATCH", `${BASE}/${uuid}`, {
      body,
    });
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
