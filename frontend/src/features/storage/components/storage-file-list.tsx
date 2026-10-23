"use client";

import { Star } from "lucide-react";

import { Checkbox } from "@/components/ui/checkbox";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { StorageAccessBadge } from "@/features/storage/components/storage-access-badge";
import {
  StorageContextMenu,
  type StorageAction,
} from "@/features/storage/components/storage-context-menu";
import { StorageFileThumbnail } from "@/features/storage/components/storage-file-thumbnail";
import { formatBytes } from "@/features/storage/lib/format";
import type { StorageObject } from "@/features/storage/types";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

// Keep the explorer's own sizing over the shadcn cell defaults (fixed
// height, no wrapping, checkbox nudge).
const headClass =
  "text-muted-foreground h-auto px-3 py-2 text-start font-medium whitespace-normal";
const cellClass =
  "px-3 py-2 whitespace-normal [&:has([role=checkbox])]:pr-3 [&>[role=checkbox]]:translate-y-0";

export function StorageFileList({
  items,
  loading,
  selected,
  canWrite,
  trash,
  onToggle,
  onOpen,
  onAction,
}: {
  items: StorageObject[];
  loading?: boolean;
  selected: Set<string>;
  canWrite: boolean;
  trash?: boolean;
  onToggle: (key: string, checked: boolean) => void;
  onOpen: (item: StorageObject) => void;
  onAction: (action: StorageAction, item: StorageObject) => void;
}) {
  const { t, locale } = useLocale();

  if (loading) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-11 w-full" />
        ))}
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border">
      <Table className="min-w-[720px]">
        <TableHeader className="bg-muted/40 text-muted-foreground border-b">
          <TableRow className="transition-none hover:bg-transparent">
            <TableHead className={cn(headClass, "w-10")} />
            <TableHead className={headClass}>{t("storage.col_name")}</TableHead>
            <TableHead className={headClass}>{t("storage.col_type")}</TableHead>
            <TableHead className={headClass}>{t("storage.col_size")}</TableHead>
            <TableHead className={headClass}>
              {t("storage.col_modified")}
            </TableHead>
            <TableHead className={headClass}>
              {t("storage.col_access")}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((item) => {
            const checked = selected.has(item.key);
            return (
              <StorageContextMenu
                key={item.key}
                object={item}
                canWrite={canWrite}
                trash={trash}
                onAction={onAction}
              >
                <TableRow
                  className={cn(
                    "hover:bg-muted/40 border-b transition-none last:border-b-0",
                    checked && "bg-accent/40",
                  )}
                  onDoubleClick={() => onOpen(item)}
                >
                  <TableCell className={cellClass}>
                    <Checkbox
                      checked={checked}
                      onCheckedChange={(value) =>
                        onToggle(item.key, value === true)
                      }
                      aria-label={item.name}
                    />
                  </TableCell>
                  <TableCell className={cellClass}>
                    <button
                      type="button"
                      className="flex max-w-md items-center gap-2 text-start"
                      onClick={() => onOpen(item)}
                    >
                      <StorageFileThumbnail
                        object={item}
                        className="size-9 shrink-0 rounded-md"
                        iconClassName="size-4"
                      />
                      <span className="truncate font-medium">{item.name}</span>
                      {item.is_starred ? (
                        <Star className="size-3.5 fill-amber-400 text-amber-400" />
                      ) : null}
                    </button>
                  </TableCell>
                  <TableCell className={cn(cellClass, "text-muted-foreground")}>
                    {t(
                      `storage.kind_${item.file_kind}` as "storage.kind_unknown",
                    )}
                  </TableCell>
                  <TableCell className={cn(cellClass, "text-muted-foreground")}>
                    {item.kind === "folder" ? "—" : formatBytes(item.size)}
                  </TableCell>
                  <TableCell className={cn(cellClass, "text-muted-foreground")}>
                    {datetime(item.updated_at, "dd.MM.yyyy HH:mm", locale)}
                  </TableCell>
                  <TableCell className={cellClass}>
                    <StorageAccessBadge access={item.access} />
                  </TableCell>
                </TableRow>
              </StorageContextMenu>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}
