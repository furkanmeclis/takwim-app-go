"use client";

import { useTheme } from "next-themes";
import { useEffect, type ReactNode } from "react";
import { Toaster, toast } from "sonner";

import { ApiError, setGlobalApiErrorHandler } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

export function ToastProvider({ children }: { children: ReactNode }) {
  const { theme } = useTheme();
  const { t } = useLocale();

  useEffect(() => {
    setGlobalApiErrorHandler((error: ApiError) => {
      if (error.isUnauthorized) {
        toast.error(t("auth.session_expired"));
        return;
      }
      if (error.isForbidden) {
        toast.error(t("common.error_forbidden"));
        return;
      }
      if (error.isNotFound) {
        toast.error(t("common.error_not_found"));
        return;
      }
      if (error.isValidation) {
        toast.error(error.message || t("common.error_validation"));
        return;
      }
      if (error.isServer) {
        toast.error(
          error.requestId
            ? `${t("common.error_server")} (${error.requestId})`
            : t("common.error_server"),
        );
        return;
      }
      toast.error(error.message || t("common.error_generic"));
    });

    return () => setGlobalApiErrorHandler(null);
  }, [t]);

  return (
    <>
      {children}
      <Toaster
        richColors
        position="top-right"
        theme={theme === "dark" ? "dark" : "light"}
      />
    </>
  );
}

export const appToast = {
  success: (message: string) => toast.success(message),
  error: (message: string) => toast.error(message),
  warning: (message: string) => toast.warning(message),
  info: (message: string) => toast.info(message),
};
