"use client";

import {
  useConfirmationFollowUpAccess,
  useConfirmationSummary,
} from "@/features/confirmations/hooks/use-confirmations";
import { createNavAdornment, type NavAdornment } from "@/features/nav-engine";
import { useLocale } from "@/providers/locale-provider";

/**
 * Sidebar badge of the confirmation list (TEC-924): upcoming visits whose
 * confirmation needs a person, with the reasons in the info popover.
 */
function useConfirmationsNavAdornment(): NavAdornment {
  const { t } = useLocale();
  const access = useConfirmationFollowUpAccess();
  const { data } = useConfirmationSummary(access);
  if (!access || !data) return { badges: [], info: null };
  return {
    badges: [{ kind: "count", value: data.attention, variant: "warning" }],
    info: {
      title: t("confirmations.nav.info_title"),
      description: t("confirmations.nav.info_description", {
        count: data.waiting,
      }),
      rows: [
        {
          label: t("confirmations.list.states.change_requested"),
          value: data.change_requested,
          tone: "warning",
        },
        {
          label: t("confirmations.list.states.unreachable"),
          value: data.unreachable,
          tone: "danger",
        },
        {
          label: t("confirmations.list.states.flagged"),
          value: data.flagged,
          tone: "danger",
        },
        {
          label: t("confirmations.nav.send_failed"),
          value: data.send_failed,
          tone: "danger",
        },
      ],
    },
  };
}

export const ConfirmationsNavAdornment = createNavAdornment(
  useConfirmationsNavAdornment,
);
