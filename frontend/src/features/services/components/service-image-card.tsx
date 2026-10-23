"use client";

import { ImageUp, Trash2 } from "lucide-react";
import { useRef, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { permissions } from "@/config/permissions";
import { MAX_SERVICE_IMAGE_BYTES } from "@/features/services/constants";
import {
  useRemoveServiceImage,
  useUploadServiceImage,
} from "@/features/services/hooks/use-services";
import type { Service } from "@/features/services/types";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const ACCEPT = "image/png,image/jpeg,image/webp";

/** Cover image of the service for the booking page and the mobile app. */
export function ServiceImageCard({ service }: { service: Service }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.services.write);
  const fileInput = useRef<HTMLInputElement>(null);
  const upload = useUploadServiceImage(service.uuid);
  const remove = useRemoveServiceImage(service.uuid);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const imageUrl = assertServiceMediaURL(service.image_url);

  return (
    <EntitySectionCard
      title={t("services.image.title")}
      actions={
        canWrite ? (
          <>
            <Input
              ref={fileInput}
              type="file"
              accept={ACCEPT}
              className="hidden"
              aria-label={t("services.image.upload")}
              onChange={(event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                if (!file) return;
                if (file.size > MAX_SERVICE_IMAGE_BYTES) {
                  appToast.error(t("services.image.too_large"));
                  return;
                }
                upload.mutate(file);
              }}
            />
            {imageUrl ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setConfirmRemove(true)}
              >
                <Trash2 className="size-4" />
                {t("services.image.remove")}
              </Button>
            ) : null}
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={upload.isPending}
              onClick={() => fileInput.current?.click()}
            >
              <ImageUp className="size-4" />
              {imageUrl
                ? t("services.image.replace")
                : t("services.image.upload")}
            </Button>
          </>
        ) : null
      }
    >
      {imageUrl ? (
        <figure className="bg-muted max-w-md overflow-hidden rounded-lg border">
          {/* eslint-disable-next-line @next/next/no-img-element -- API media URL, no image optimizer */}
          <img
            src={imageUrl}
            alt={service.name}
            className="aspect-[4/3] w-full object-cover"
            loading="lazy"
          />
        </figure>
      ) : (
        <EmptyState
          className="py-8"
          title={t("services.image.empty")}
          description={t("services.image.hint")}
        />
      )}

      <ConfirmDialog
        open={confirmRemove}
        title={t("services.image.remove_title")}
        description={t("services.image.remove_description")}
        confirmLabel={t("services.image.remove")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setConfirmRemove(false)}
        onConfirm={() =>
          remove.mutate(undefined, {
            onSettled: () => setConfirmRemove(false),
          })
        }
      />
    </EntitySectionCard>
  );
}
