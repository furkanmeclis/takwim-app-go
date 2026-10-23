"use client";

import { Download } from "lucide-react";
import { useState } from "react";
import Link from "next/link";

import {
  EXPORT_PATHS,
  type ExportFormat,
  type ExportJob,
  type IoResource,
} from "@/features/io/types";
import { exportsService } from "@/features/io/services/exports.service";
import { useApiScope } from "@/features/tenant/hooks/use-api-scope";
import type { ApiScope } from "@/lib/api/scoped-request";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type ExportMenuProps = {
  resource: IoResource;
  query?: Record<string, string | undefined>;
  disabled?: boolean;
};

const FORMATS: ExportFormat[] = ["pdf", "xlsx", "csv", "json"];

const POLL_INTERVAL_MS = 1500;
const POLL_ATTEMPTS = 80;

/**
 * Organization exports have no jobs page: wait for the job, then download.
 * Resolves false when the job failed or did not finish in time.
 */
async function waitAndDownload(job: ExportJob, scope: ApiScope) {
  let current = job;
  for (let i = 0; i < POLL_ATTEMPTS; i += 1) {
    if (current.status === "completed") {
      await exportsService.download(current, scope);
      return true;
    }
    if (current.status === "failed") return false;
    await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
    current = await exportsService.get(current.uuid, scope);
  }
  return false;
}

export function ExportMenu({ resource, query, disabled }: ExportMenuProps) {
  const { t, locale } = useLocale();
  const scope = useApiScope();
  const [pending, setPending] = useState<ExportFormat | null>(null);

  const handleExport = async (format: ExportFormat) => {
    setPending(format);
    try {
      const cleaned: Record<string, string> = {};
      if (query) {
        for (const [key, value] of Object.entries(query)) {
          if (value) cleaned[key] = value;
        }
      }
      const job = await exportsService.request(
        EXPORT_PATHS[resource],
        {
          format,
          query: cleaned,
          locale,
        },
        scope,
      );
      if (scope.kind === "platform") {
        appToast.success(t("exports.toast.queued"));
        return;
      }
      appToast.info(t("exports.toast.preparing"));
      const ok = await waitAndDownload(job, scope);
      if (ok) appToast.success(t("exports.toast.ready"));
      else appToast.error(t("exports.toast.download_failed"));
    } catch {
      appToast.error(t("exports.toast.failed"));
    } finally {
      setPending(null);
    }
  };

  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="size-8"
              disabled={disabled || Boolean(pending)}
              aria-label={t("exports.menu_label")}
            >
              <Download className="size-4" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent side="bottom">{t("exports.menu_label")}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end">
        {FORMATS.map((format) => (
          <DropdownMenuItem
            key={format}
            disabled={pending === format}
            onClick={() => void handleExport(format)}
          >
            {t(`exports.formats.${format}`)}
          </DropdownMenuItem>
        ))}
        {scope.kind === "platform" ? (
          <DropdownMenuItem asChild>
            <Link href={routes.platform.exports.root}>
              {t("exports.view_jobs")}
            </Link>
          </DropdownMenuItem>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
