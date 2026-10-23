"use client";

import { ArrowLeft, ArrowRight, ImageUp, Trash2 } from "lucide-react";
import { useRef, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntitySectionCard } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { permissions } from "@/config/permissions";
import {
  useDeleteBranchImage,
  useReorderBranchImages,
  useUploadBranchImage,
} from "@/features/branches/hooks/use-branches";
import type { BranchImage } from "@/features/branches/types";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const ACCEPT = "image/png,image/jpeg,image/webp";
const MAX_BYTES = 5 * 1024 * 1024;
const MAX_IMAGES = 12;

/**
 * Branch photos (TEC-888) for the booking page and the mobile app. The
 * first photo is the cover; arrows move a photo, the API stores the order.
 */
export function BranchGalleryCard({
  branchUuid,
  images,
}: {
  branchUuid: string;
  images: BranchImage[];
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.branches.write);
  const fileInput = useRef<HTMLInputElement>(null);
  const upload = useUploadBranchImage(branchUuid);
  const reorder = useReorderBranchImages(branchUuid);
  const remove = useDeleteBranchImage(branchUuid);
  const [pendingDelete, setPendingDelete] = useState<BranchImage | null>(null);
  const full = images.length >= MAX_IMAGES;

  const move = (index: number, delta: number) => {
    const order = images.map((image) => image.uuid);
    const target = index + delta;
    if (target < 0 || target >= order.length) return;
    [order[index], order[target]] = [order[target], order[index]];
    reorder.mutate(order);
  };

  return (
    <EntitySectionCard
      title={t("branches.gallery.title")}
      badge={images.length}
      actions={
        canWrite ? (
          <>
            <Input
              ref={fileInput}
              type="file"
              accept={ACCEPT}
              className="hidden"
              aria-label={t("branches.gallery.upload")}
              onChange={(event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                if (!file) return;
                if (file.size > MAX_BYTES) {
                  appToast.error(t("branches.gallery.too_large"));
                  return;
                }
                upload.mutate(file);
              }}
            />
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={upload.isPending || full}
              onClick={() => fileInput.current?.click()}
            >
              <ImageUp className="size-4" />
              {t("branches.gallery.upload")}
            </Button>
          </>
        ) : null
      }
    >
      {images.length === 0 ? (
        <EmptyState
          className="py-8"
          title={t("branches.gallery.empty")}
          description={t("branches.gallery.hint")}
        />
      ) : (
        <div className="space-y-3">
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
            {images.map((image, index) => (
              <figure
                key={image.uuid}
                className="bg-muted relative overflow-hidden rounded-lg border"
              >
                {/* eslint-disable-next-line @next/next/no-img-element -- API media URL, no image optimizer */}
                <img
                  src={assertServiceMediaURL(image.public_url) ?? undefined}
                  alt={image.filename}
                  className="aspect-[4/3] w-full object-cover"
                  loading="lazy"
                />
                {index === 0 ? (
                  <Badge className="absolute top-2 left-2">
                    {t("branches.gallery.cover")}
                  </Badge>
                ) : null}
                {canWrite ? (
                  <figcaption className="bg-background/90 flex items-center justify-between gap-1 p-1">
                    <div className="flex gap-1">
                      <Button
                        type="button"
                        size="icon"
                        variant="ghost"
                        className="size-8"
                        disabled={index === 0 || reorder.isPending}
                        aria-label={t("branches.gallery.move_left")}
                        onClick={() => move(index, -1)}
                      >
                        <ArrowLeft className="size-4" />
                      </Button>
                      <Button
                        type="button"
                        size="icon"
                        variant="ghost"
                        className="size-8"
                        disabled={
                          index === images.length - 1 || reorder.isPending
                        }
                        aria-label={t("branches.gallery.move_right")}
                        onClick={() => move(index, 1)}
                      >
                        <ArrowRight className="size-4" />
                      </Button>
                    </div>
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      className="text-destructive size-8"
                      aria-label={t("branches.gallery.delete")}
                      onClick={() => setPendingDelete(image)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </figcaption>
                ) : null}
              </figure>
            ))}
          </div>
          <p className="text-muted-foreground text-xs">
            {t("branches.gallery.hint")}
          </p>
        </div>
      )}

      <ConfirmDialog
        open={pendingDelete !== null}
        title={t("branches.gallery.delete_title")}
        description={t("branches.gallery.delete_description")}
        confirmLabel={t("branches.gallery.delete")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setPendingDelete(null)}
        onConfirm={() =>
          pendingDelete &&
          remove.mutate(pendingDelete.uuid, {
            onSettled: () => setPendingDelete(null),
          })
        }
      />
    </EntitySectionCard>
  );
}
