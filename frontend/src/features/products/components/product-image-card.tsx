"use client";

import { ImageUp, Trash2 } from "lucide-react";
import { useRef, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { permissions } from "@/config/permissions";
import { MAX_PRODUCT_IMAGE_BYTES } from "@/features/products/constants";
import {
  useRemoveProductImage,
  useUploadProductImage,
} from "@/features/products/hooks/use-products";
import type { Product } from "@/features/products/types";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const ACCEPT = "image/png,image/jpeg,image/webp";

/** Product photo for the online shop and the mobile app. */
export function ProductImageCard({ product }: { product: Product }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.products.write);
  const fileInput = useRef<HTMLInputElement>(null);
  const upload = useUploadProductImage(product.uuid);
  const remove = useRemoveProductImage(product.uuid);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const imageUrl = assertServiceMediaURL(product.image_url);

  return (
    <EntitySectionCard
      title={t("products.image.title")}
      actions={
        canWrite ? (
          <>
            <Input
              ref={fileInput}
              type="file"
              accept={ACCEPT}
              className="hidden"
              aria-label={t("products.image.upload")}
              onChange={(event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                if (!file) return;
                if (file.size > MAX_PRODUCT_IMAGE_BYTES) {
                  appToast.error(t("products.image.too_large"));
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
                {t("products.image.remove")}
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
                ? t("products.image.replace")
                : t("products.image.upload")}
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
            alt={product.name}
            className="aspect-[4/3] w-full object-cover"
            loading="lazy"
          />
        </figure>
      ) : (
        <EmptyState
          className="py-8"
          title={t("products.image.empty")}
          description={t("products.image.hint")}
        />
      )}

      <ConfirmDialog
        open={confirmRemove}
        title={t("products.image.remove_title")}
        description={t("products.image.remove_description")}
        confirmLabel={t("products.image.remove")}
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
