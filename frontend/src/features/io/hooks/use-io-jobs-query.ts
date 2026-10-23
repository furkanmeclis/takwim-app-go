"use client";

import { useQuery } from "@tanstack/react-query";

import { ioKeys } from "@/features/io/hooks/query-keys";
import { exportsService } from "@/features/io/services/exports.service";
import { importsService } from "@/features/io/services/imports.service";

export function useExportJob(uuid: string, enabled = true) {
  return useQuery({
    queryKey: ioKeys.exports.detail(uuid),
    queryFn: () => exportsService.get(uuid),
    enabled: Boolean(uuid) && enabled,
  });
}

export function useImportJob(uuid: string, enabled = true) {
  return useQuery({
    queryKey: ioKeys.imports.detail(uuid),
    queryFn: () => importsService.get(uuid),
    enabled: Boolean(uuid) && enabled,
  });
}
