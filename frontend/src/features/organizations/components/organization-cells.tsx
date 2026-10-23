"use client";

import { StatusChip } from "@/components/common/status-chip";
import {
  ACCESS_LABEL_KEYS,
  ACCESS_TONE,
  STATUS_LABEL_KEYS,
  STATUS_TONE,
} from "@/features/organizations/constants";
import type {
  OrganizationAccessState,
  OrganizationStatus,
} from "@/features/organizations/types";
import { useLocale } from "@/providers/locale-provider";

export function OrganizationStatusChip({
  status,
}: {
  status: OrganizationStatus;
}) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(STATUS_LABEL_KEYS[status])}
      tone={STATUS_TONE[status]}
    />
  );
}

export function OrganizationAccessChip({
  state,
}: {
  state: OrganizationAccessState;
}) {
  const { t } = useLocale();
  return (
    <StatusChip label={t(ACCESS_LABEL_KEYS[state])} tone={ACCESS_TONE[state]} />
  );
}

export function ownerLabel(owner: {
  name: string;
  surname: string;
  email: string;
}) {
  return `${owner.name} ${owner.surname}`.trim() || owner.email;
}
