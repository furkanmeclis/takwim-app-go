"use client";

import { EmptyState } from "@/components/common/empty-state";
import { useLocale } from "@/providers/locale-provider";

export default function BusinessNotFound() {
  const { t } = useLocale();
  return (
    <main className="mx-auto flex min-h-svh w-full max-w-3xl items-center px-4">
      <EmptyState
        className="w-full"
        title={t("business_page.not_found_title")}
        description={t("business_page.not_found_description")}
      />
    </main>
  );
}
