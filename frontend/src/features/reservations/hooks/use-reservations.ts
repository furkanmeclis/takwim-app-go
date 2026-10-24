"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { cashKeys } from "@/features/cash/hooks/query-keys";
import { reservationsKeys } from "@/features/reservations/hooks/query-keys";
import { reportReservationError } from "@/features/reservations/lib/errors";
import { reservationsService } from "@/features/reservations/services/reservations.service";
import type {
  AvailabilitySearchRequest,
  AgendaParams,
  CalendarParams,
  ListReservationsParams,
  ReservationAction,
  ReservationActionRequest,
  ReservationCollectionRequest,
  ReservationCreateRequest,
  ReservationNotesRequest,
  ReservationRescheduleRequest,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useReservationsList(params: ListReservationsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationsKeys.list(slug, params),
    queryFn: () => reservationsService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useReservationsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationsKeys.meta(slug),
    queryFn: () => reservationsService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useReservationsFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationsKeys.facets(slug),
    queryFn: () => reservationsService.facets(slug),
    staleTime: 60_000,
  });
}

export function useReservationCalendar(params: CalendarParams | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationsKeys.calendar(slug, params ?? {}),
    queryFn: () => reservationsService.calendar(slug, params!),
    enabled: Boolean(params),
    placeholderData: keepPreviousData,
  });
}

export function useReservationAgenda(params: AgendaParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationsKeys.agenda(slug, params),
    queryFn: () => reservationsService.agenda(slug, params),
    placeholderData: keepPreviousData,
  });
}

export function useReservation(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationsKeys.detail(slug, uuid ?? ""),
    queryFn: () => reservationsService.get(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

/** Free starts of one or more days (the API plans; null = not asked yet). */
export function useAvailability(body: AvailabilitySearchRequest | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationsKeys.availability(slug, body ?? {}),
    queryFn: () => reservationsService.availability(slug, body!),
    enabled: Boolean(body),
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });
}

function useInvalidateReservations() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: reservationsKeys.all(slug) }),
      queryClient.invalidateQueries({
        queryKey: ["tenant", slug, "availability"],
      }),
    ]);
}

export function useCreateReservation() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: (body: ReservationCreateRequest) =>
      reservationsService.create(slug, body),
    onSuccess: () => {
      appToast.success(t("reservations.toast.created"));
      void invalidate();
    },
    // A taken time refreshes the slots so the picker shows what is left.
    onError: () => void invalidate(),
  });
}

const ACTION_TOAST_KEYS: Record<ReservationAction, string> = {
  accept: "reservations.toast.accepted",
  reject: "reservations.toast.rejected",
  cancel: "reservations.toast.cancelled",
  start: "reservations.toast.started",
  complete: "reservations.toast.completed",
  "no-show": "reservations.toast.no_show",
};

export function useReservationAction() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: ({
      uuid,
      action,
      body,
    }: {
      uuid: string;
      action: ReservationAction;
      body: ReservationActionRequest;
    }) => reservationsService.action(slug, uuid, action, body),
    onSuccess: (_, { action }) => {
      appToast.success(t(ACTION_TOAST_KEYS[action]));
      void invalidate();
    },
    onError: (error) => {
      reportReservationError(error, t);
      void invalidate();
    },
  });
}

export function useRescheduleReservation() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: ReservationRescheduleRequest;
    }) => reservationsService.reschedule(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("reservations.toast.rescheduled"));
      void invalidate();
    },
    onError: (error) => {
      reportReservationError(error, t);
      void invalidate();
    },
  });
}

export function useUpdateReservationNotes() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: ReservationNotesRequest;
    }) => reservationsService.updateNotes(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("reservations.toast.notes_saved"));
      void invalidate();
    },
  });
}

/** Money of one visit: expected, paid, entries and online attempts. */
export function useReservationPayments(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationsKeys.payments(slug, uuid),
    queryFn: () => reservationsService.payments(slug, uuid),
    enabled: Boolean(uuid),
  });
}

/** Records an on-site payment; the visit, history and money refresh. */
export function useCollectReservationPayment(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: (body: ReservationCollectionRequest) =>
      reservationsService.collect(slug, uuid, body),
    onSuccess: (view) => {
      queryClient.setQueryData(reservationsKeys.payments(slug, uuid), view);
      appToast.success(t("reservations.toast.payment_collected"));
      void invalidate();
      void queryClient.invalidateQueries({ queryKey: cashKeys.all(slug) });
    },
  });
}
