"use client";

import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, Download, Loader2 } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  EntityDetail,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  exportDownloadFilename,
  formatExportFormat,
  formatExportStatus,
  resourceLabelKey,
} from "@/features/io/lib/display";
import { exportsService } from "@/features/io/services/exports.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useApiScope } from "@/features/tenant/hooks/use-api-scope";
import { scopeKey } from "@/lib/api/scoped-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type DownloadState = "idle" | "downloading" | "done" | "failed";

const STATE_KEYS = {
  idle: "exports.tenant_download.state_idle",
  downloading: "exports.tenant_download.state_downloading",
  done: "exports.tenant_download.state_done",
  failed: "exports.tenant_download.state_failed",
} as const satisfies Record<DownloadState, string>;

/**
 * `/t/{slug}/exports/{uuid}/download` — the action link of an organization's
 * "export ready" notification. A link cannot carry `X-Organization`, so this
 * page downloads the file through `/v1/tenant/exports/{uuid}/download` with
 * the organization of the URL as soon as the job is ready, and offers the
 * button again. The API checks membership and that the job is the caller's.
 */
export function TenantExportDownloadPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const scope = useApiScope();
  const [state, setState] = useState<DownloadState>("idle");
  const started = useRef(false);

  const jobQuery = useQuery({
    queryKey: ["tenant", "exports", scopeKey(scope), uuid],
    queryFn: () => exportsService.get(uuid, scope),
    enabled: Boolean(uuid) && scope.kind === "tenant",
    retry: false,
  });
  const job = jobQuery.data;

  const download = useCallback(async () => {
    if (!job) return;
    setState("downloading");
    try {
      await exportsService.download(job, scope);
      setState("done");
    } catch {
      setState("failed");
      appToast.error(t("exports.toast.download_failed"));
    }
  }, [job, scope, t]);

  // One click from the notification: start the download once the job is
  // known to be ready.
  useEffect(() => {
    if (started.current || job?.status !== "completed") return;
    started.current = true;
    void download();
  }, [download, job?.status]);

  const resourceName = job
    ? (() => {
        const key = resourceLabelKey(job.resource);
        return key ? t(`exports.${key}`) : job.resource;
      })()
    : "";
  const title = t("exports.tenant_download.title");

  return (
    <EntityPage
      title={title}
      description={t("exports.tenant_download.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        job?.status === "completed" ? (
          <Button
            size="sm"
            disabled={state === "downloading"}
            onClick={() => void download()}
          >
            {state === "downloading" ? (
              <Loader2 className="size-4 animate-spin" />
            ) : state === "done" ? (
              <CheckCircle2 className="size-4" />
            ) : (
              <Download className="size-4" />
            )}
            {state === "done" || state === "failed"
              ? t("exports.tenant_download.again")
              : t("exports.download")}
          </Button>
        ) : null
      }
    >
      {jobQuery.isLoading ? <Loading label={t("common.loading")} /> : null}
      {jobQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("exports.detail.not_found")}
          onRetry={() => void jobQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {job ? (
        <EntitySectionCard title={t("exports.detail.download_section")}>
          <p className="text-muted-foreground mb-4 text-sm" aria-live="polite">
            {job.status === "completed"
              ? t(STATE_KEYS[state])
              : t("exports.tenant_download.not_ready")}
          </p>
          <EntityDetail
            sections={[
              {
                id: "summary",
                fields: [
                  {
                    key: "resource",
                    label: t("exports.columns.resource"),
                    value: resourceName,
                  },
                  {
                    key: "format",
                    label: t("exports.columns.format"),
                    value: formatExportFormat(t, job.format),
                  },
                  {
                    key: "status",
                    label: t("exports.columns.status"),
                    value: (
                      <Badge
                        variant={
                          job.status === "completed"
                            ? "default"
                            : job.status === "failed"
                              ? "danger"
                              : "outline"
                        }
                      >
                        {formatExportStatus(t, job.status)}
                      </Badge>
                    ),
                  },
                  {
                    key: "row_count",
                    label: t("exports.columns.rows"),
                    value: job.row_count,
                  },
                  {
                    key: "filename",
                    label: t("exports.detail.fields.filename"),
                    value: exportDownloadFilename(
                      job.resource,
                      job.format,
                      new Date(job.created_at),
                    ),
                  },
                ],
              },
            ]}
          />
        </EntitySectionCard>
      ) : null}
    </EntityPage>
  );
}
