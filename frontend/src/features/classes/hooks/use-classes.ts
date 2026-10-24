"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { classesKeys } from "@/features/classes/hooks/query-keys";
import {
  localizeClassError,
  reportClassError,
} from "@/features/classes/lib/errors";
import { classesService } from "@/features/classes/services/classes.service";
import type {
  ClassScheduleInput,
  ClassSessionChangeInput,
  ListClassSessionsParams,
  ListClassesParams,
} from "@/features/classes/types";
import { reservationsKeys } from "@/features/reservations/hooks/query-keys";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useClasses(params: ListClassesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: classesKeys.list(slug, params),
    queryFn: () => classesService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useClassFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: classesKeys.facets(slug),
    queryFn: () => classesService.facets(slug),
    staleTime: 30_000,
  });
}

/** Choices of the class form; only fetched while the form is open. */
export function useClassForm(enabled: boolean) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: classesKeys.form(slug),
    queryFn: () => classesService.form(slug),
    staleTime: 30_000,
    enabled,
  });
}

export function useClassSessions(params: ListClassSessionsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: classesKeys.sessions(slug, params),
    queryFn: () => classesService.sessions(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useClassSession(uuid: string, date: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: classesKeys.session(slug, uuid, date),
    queryFn: () => classesService.session(slug, uuid, date),
  });
}

/** A timetable write changes the lists, the sessions and the free times. */
function useInvalidateClasses() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: classesKeys.all(slug) }),
      queryClient.invalidateQueries({ queryKey: reservationsKeys.all(slug) }),
      queryClient.invalidateQueries({
        queryKey: ["tenant", slug, "availability"],
      }),
    ]);
}

export function useSaveClass() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateClasses();
  return useMutation({
    mutationFn: async ({
      uuid,
      body,
    }: {
      uuid?: string;
      body: ClassScheduleInput;
    }) => {
      try {
        return await (uuid
          ? classesService.update(slug, uuid, body)
          : classesService.create(slug, body));
      } catch (error) {
        throw localizeClassError(error, t);
      }
    },
    onSuccess: (_row, { uuid }) => {
      appToast.success(
        t(uuid ? "classes.toast.updated" : "classes.toast.created"),
      );
      void invalidate();
    },
    onError: (error) => reportClassError(error, t),
  });
}

export function useDeleteClass() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateClasses();
  return useMutation({
    mutationFn: (uuid: string) => classesService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("classes.toast.deleted"));
      void invalidate();
    },
    onError: (error) => reportClassError(error, t),
  });
}

/**
 * Cancels a session with its bookings (TEC-989). The answer carries the
 * session as it is now, with the run that can be rolled back.
 */
export function useCancelClassSession() {
  const { slug } = useTenant();
  const { t, locale } = useLocale();
  const queryClient = useQueryClient();
  const invalidate = useInvalidateClasses();
  return useMutation({
    mutationFn: async ({
      uuid,
      date,
      reason,
    }: {
      uuid: string;
      date: string;
      reason: string;
    }) => {
      try {
        return await classesService.cancelSession(slug, uuid, date, {
          reason,
          locale,
        });
      } catch (error) {
        throw localizeClassError(error, t);
      }
    },
    onSuccess: (result, { uuid, date }) => {
      queryClient.setQueryData(
        classesKeys.session(slug, uuid, date),
        result.session,
      );
      const run = result.cancellation;
      if (!run) {
        appToast.success(t("classes.toast.session_cancelled"));
      } else if (run.failed + run.skipped > 0) {
        appToast.warning(
          t("classes.toast.session_cancelled_partial", {
            succeeded: run.succeeded,
            total: run.total,
          }),
        );
      } else {
        appToast.success(
          t("classes.toast.session_cancelled_with_bookings", {
            count: run.succeeded,
          }),
        );
      }
      void invalidate();
    },
    onError: (error) => reportClassError(error, t),
  });
}

/**
 * Rolls a session cancel back: the session opens again and its bookings
 * are taken back (a booking changed since stays cancelled).
 */
export function useUndoClassSessionCancel() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateClasses();
  return useMutation({
    mutationFn: ({ jobUuid }: { jobUuid: string }) =>
      classesService.undoCancel(slug, jobUuid),
    onSuccess: (job) => {
      if (job.status === "rolled_back") {
        appToast.success(t("classes.toast.cancel_undone"));
      } else {
        appToast.warning(t("classes.toast.cancel_undone_partial"));
      }
      void invalidate();
    },
    onError: (error) => reportClassError(error, t),
  });
}

export function useChangeClassSession() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const invalidate = useInvalidateClasses();
  return useMutation({
    mutationFn: async ({
      uuid,
      date,
      body,
    }: {
      uuid: string;
      date: string;
      body: ClassSessionChangeInput;
    }) => {
      try {
        return await classesService.changeSession(slug, uuid, date, body);
      } catch (error) {
        throw localizeClassError(error, t);
      }
    },
    onSuccess: (detail, { uuid, date, body }) => {
      queryClient.setQueryData(classesKeys.session(slug, uuid, date), detail);
      appToast.success(
        t(
          body.cancelled
            ? "classes.toast.session_cancelled"
            : "classes.toast.session_saved",
        ),
      );
      void invalidate();
    },
    onError: (error) => reportClassError(error, t),
  });
}
