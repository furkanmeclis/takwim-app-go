"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { integrationsService } from "@/features/integrations/services/integrations.service";
import type {
  ConnectIntegrationRequest,
  ListIntegrationJobsParams,
  ListIntegrationsParams,
  UpdateIntegrationRequest,
} from "@/features/integrations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const integrationKeys = {
  all: (slug: string) => ["tenant", slug, "integrations"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...integrationKeys.all(slug), "list", params] as const,
  meta: (slug: string) => [...integrationKeys.all(slug), "meta"] as const,
  jobs: (slug: string, account: string, params: Record<string, unknown>) =>
    [...integrationKeys.all(slug), "jobs", account, params] as const,
  job: (slug: string, uuid: string) =>
    [...integrationKeys.all(slug), "job", uuid] as const,
};

function useInvalidateIntegrations() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: integrationKeys.all(slug) });
}

export function useIntegrations(params: ListIntegrationsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: integrationKeys.list(slug, params),
    queryFn: () => integrationsService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useIntegrationsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: integrationKeys.meta(slug),
    queryFn: () => integrationsService.meta(slug),
    staleTime: 10 * 60_000,
  });
}

export function useConnectIntegration() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateIntegrations();
  return useMutation({
    mutationFn: (body: ConnectIntegrationRequest) =>
      integrationsService.connect(slug, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("integrations.toast.connected"));
      void invalidate();
    },
  });
}

export function useUpdateIntegration() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateIntegrations();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: UpdateIntegrationRequest;
    }) => integrationsService.update(slug, uuid, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("integrations.toast.updated"));
      void invalidate();
    },
  });
}

export function useTestIntegration() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateIntegrations();
  return useMutation({
    mutationFn: (uuid: string) => integrationsService.test(slug, uuid),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("integrations.toast.test_ok"));
    },
    onError: () => {
      appToast.error(t("integrations.toast.test_failed"));
    },
    onSettled: () => {
      void invalidate();
    },
  });
}

export function useSetIntegrationStatus() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateIntegrations();
  return useMutation({
    mutationFn: ({
      uuid,
      action,
    }: {
      uuid: string;
      action: "enable" | "disable";
    }) => integrationsService.setStatus(slug, uuid, action),
    onSuccess: (_, variables) => {
      appToast.success(
        t(
          variables.action === "disable"
            ? "integrations.toast.disabled"
            : "integrations.toast.enabled",
        ),
      );
      void invalidate();
    },
  });
}

/** An account's jobs, or the whole organization's when account is null. */
export function useIntegrationJobs(
  account: string | null,
  params: ListIntegrationJobsParams,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: integrationKeys.jobs(slug, account ?? "*", params),
    queryFn: () => integrationsService.jobs(slug, account, params),
    placeholderData: (previous) => previous,
  });
}

export function useIntegrationJob(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: integrationKeys.job(slug, uuid ?? ""),
    queryFn: () => integrationsService.job(slug, uuid ?? ""),
    enabled: Boolean(uuid),
  });
}

/** "Yeniden dene": the backend checks the provider first and only resends work that was not done. */
export function useRetryIntegrationJob() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateIntegrations();
  return useMutation({
    mutationFn: (uuid: string) => integrationsService.retry(slug, uuid),
    meta: { silent: true },
    onSuccess: (result) => {
      const key =
        result.action === "already_done"
          ? "integrations.jobs.toast.already_done"
          : result.action === "in_progress"
            ? "integrations.jobs.toast.in_progress"
            : "integrations.jobs.toast.resubmitted";
      appToast.success(t(key));
    },
    onError: (error) => {
      const code = error instanceof ApiError ? error.code : "";
      appToast.error(
        t(
          code === "INTEGRATION_JOB_UNVERIFIED"
            ? "integrations.jobs.toast.unverified"
            : code === "INTEGRATION_JOB_BUSY"
              ? "integrations.jobs.toast.busy"
              : code === "INTEGRATION_JOB_NOT_RETRYABLE"
                ? "integrations.jobs.toast.not_retryable"
                : "integrations.jobs.toast.retry_failed",
        ),
      );
    },
    onSettled: () => {
      void invalidate();
    },
  });
}
