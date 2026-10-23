"use client";

import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { ioKeys } from "@/features/io/hooks/query-keys";
import { exportsService } from "@/features/io/services/exports.service";
import { useLocale } from "@/providers/locale-provider";

type ExportPdfPreviewProps = {
  uuid: string;
  enabled?: boolean;
};

export function ExportPdfPreview({
  uuid,
  enabled = true,
}: ExportPdfPreviewProps) {
  const { t } = useLocale();

  const previewQuery = useQuery({
    queryKey: [...ioKeys.exports.detail(uuid), "preview", "pdf"] as const,
    queryFn: async () => {
      const { blob } = await exportsService.fetchFile(uuid);
      return URL.createObjectURL(blob);
    },
    enabled,
    staleTime: 5 * 60 * 1000,
    retry: 1,
  });

  useEffect(() => {
    const url = previewQuery.data;
    return () => {
      if (url) URL.revokeObjectURL(url);
    };
  }, [previewQuery.data]);

  if (previewQuery.isLoading) {
    return <Loading label={t("exports.detail.preview_loading")} />;
  }

  if (previewQuery.isError || !previewQuery.data) {
    return (
      <ErrorState
        title={t("exports.detail.preview_failed_title")}
        description={t("exports.detail.preview_failed_description")}
        onRetry={() => void previewQuery.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  return (
    <iframe
      src={previewQuery.data}
      title={t("exports.detail.preview_pdf_title")}
      className="bg-muted h-[min(70vh,640px)] w-full rounded-md border"
    />
  );
}

export function isPdfExportFormat(format: string) {
  return format.toLowerCase() === "pdf";
}
