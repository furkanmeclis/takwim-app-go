"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { cashKeys } from "@/features/cash/hooks/query-keys";
import { financeKeys } from "@/features/finance/hooks/query-keys";
import { financeService } from "@/features/finance/services/finance.service";
import type {
  FinanceCategoryCreateRequest,
  FinanceCategoryPatchRequest,
  FinanceEntryCorrectRequest,
  FinanceEntryCreateRequest,
  FinanceEntryPatchRequest,
  FinancePeriod,
  ListFinanceCategoriesParams,
  ListFinanceEntriesParams,
} from "@/features/finance/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { download } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useFinanceCategories(
  params: ListFinanceCategoriesParams,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.categories(slug, params),
    queryFn: () => financeService.listCategories(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useFinanceEntries(params: ListFinanceEntriesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.entryList(slug, params),
    queryFn: () => financeService.listEntries(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useFinanceEntriesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.entriesMeta(slug),
    queryFn: () => financeService.entriesMeta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useFinanceFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.facets(slug),
    queryFn: () => financeService.facets(slug),
    staleTime: 60_000,
  });
}

export function useFinanceEntry(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.entry(slug, uuid),
    queryFn: () => financeService.getEntry(slug, uuid),
  });
}

export function useFinanceEntryActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.activity(slug, uuid, params),
    queryFn: () => financeService.activity(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

export function useFinanceSummary(period: FinancePeriod) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.summary(slug, period),
    queryFn: () => financeService.summary(slug, period),
    placeholderData: (previous) => previous,
  });
}

/** A record moves money: refresh the finance lists and the cash book. */
function useInvalidateFinance() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: financeKeys.all(slug) }),
      queryClient.invalidateQueries({ queryKey: cashKeys.all(slug) }),
    ]);
}

export function useCreateFinanceEntry() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFinance();
  return useMutation({
    mutationFn: (body: FinanceEntryCreateRequest) =>
      financeService.createEntry(slug, body),
    onSuccess: (entry) => {
      appToast.success(
        t(
          entry.kind === "income"
            ? "finance.toast.income_created"
            : "finance.toast.expense_created",
        ),
      );
      void invalidate();
    },
  });
}

export function useUpdateFinanceEntry(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFinance();
  return useMutation({
    mutationFn: (body: FinanceEntryPatchRequest) =>
      financeService.updateEntry(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("finance.toast.updated"));
      void invalidate();
    },
  });
}

export function useReverseFinanceEntry() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFinance();
  return useMutation({
    mutationFn: ({ uuid, reason }: { uuid: string; reason: string }) =>
      financeService.reverseEntry(slug, uuid, reason),
    onSuccess: () => {
      appToast.success(t("finance.toast.reversed"));
      void invalidate();
    },
  });
}

export function useCorrectFinanceEntry(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFinance();
  return useMutation({
    mutationFn: (body: FinanceEntryCorrectRequest) =>
      financeService.correctEntry(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("finance.toast.corrected"));
      void invalidate();
    },
  });
}

export function useUploadFinanceDocument(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFinance();
  return useMutation({
    mutationFn: (file: File) => financeService.uploadDocument(slug, uuid, file),
    onSuccess: () => {
      appToast.success(t("finance.toast.document_attached"));
      void invalidate();
    },
  });
}

export function useRemoveFinanceDocument(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFinance();
  return useMutation({
    mutationFn: () => financeService.removeDocument(slug, uuid),
    onSuccess: () => {
      appToast.success(t("finance.toast.document_removed"));
      void invalidate();
    },
  });
}

/** Streams the document through the API and saves it with its name. */
export function useDownloadFinanceDocument(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  return useMutation({
    mutationFn: async (fallbackName: string) => {
      const { blob, filename } = await financeService.downloadDocument(
        slug,
        uuid,
      );
      download(blob, filename || fallbackName);
    },
    onError: () => appToast.error(t("finance.document.download_failed")),
  });
}

export function useSaveFinanceCategory() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFinance();
  return useMutation({
    mutationFn: ({
      uuid,
      create,
      patch,
    }: {
      uuid?: string;
      create?: FinanceCategoryCreateRequest;
      patch?: FinanceCategoryPatchRequest;
    }) =>
      uuid
        ? financeService.updateCategory(slug, uuid, patch ?? {})
        : financeService.createCategory(slug, create!),
    onSuccess: (_category, { uuid }) => {
      appToast.success(
        t(
          uuid
            ? "finance.toast.category_updated"
            : "finance.toast.category_created",
        ),
      );
      void invalidate();
    },
  });
}

export function useDeleteFinanceCategory() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFinance();
  return useMutation({
    mutationFn: (uuid: string) => financeService.deleteCategory(slug, uuid),
    onSuccess: () => {
      appToast.success(t("finance.toast.category_deleted"));
      void invalidate();
    },
  });
}
