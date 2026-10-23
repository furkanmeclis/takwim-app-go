"use client";

import { ChevronLeft, ChevronRight, RefreshCw } from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { EntityCreateButton, EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CalendarGrid,
  type CalendarView,
  type GridPoint,
} from "@/features/reservations/components/calendar/calendar-grid";
import {
  MoveDialog,
  type MoveTarget,
} from "@/features/reservations/components/calendar/move-dialog";
import {
  RESERVATIONS_RESOURCE,
  STATUS,
} from "@/features/reservations/constants";
import { useReservationCalendar } from "@/features/reservations/hooks/use-reservations";
import {
  addDays,
  clock,
  weekStart,
} from "@/features/reservations/lib/calendar-time";
import {
  READ_ANY,
  useCanWriteReservations,
} from "@/features/reservations/lib/permissions";
import type {
  CalendarBy,
  CalendarItem,
  CalendarParams,
} from "@/features/reservations/types";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useSelectedBranch } from "@/features/tenant/hooks/use-selected-branch";
import { tenantResourceChannel } from "@/features/tenant/lib/live";
import { useChannel } from "@/hooks/use-realtime";
import { dateKeyInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const ALL = "all";

const LEGEND = [
  "requested",
  "active",
  "in_service",
  "completed",
  "no_show",
] as const;

/**
 * Day / week calendar of a branch by person or resource, or of every branch.
 * Free space starts a booking at that time; a visit dragged elsewhere is
 * moved after a confirmation (the API checks the new time). Changes made
 * elsewhere arrive live.
 */
export function ReservationCalendarPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const { can } = usePermission();
  const canWrite = useCanWriteReservations();
  const branches = useAccessibleBranches();
  const [selectedBranch] = useSelectedBranch(slug);
  const zone = organization?.timezone ?? "Europe/Istanbul";

  const [date, setDate] = useState(() => dateKeyInZone(new Date(), zone));
  const [view, setView] = useState<CalendarView>("day");
  const [by, setBy] = useState<Exclude<CalendarBy, "branch">>("staff");
  const [branchChoice, setBranchChoice] = useState<string | null>(null);
  const [columnKey, setColumnKey] = useState(ALL);
  const [includeCancelled, setIncludeCancelled] = useState(false);
  const [moveTarget, setMoveTarget] = useState<MoveTarget | null>(null);

  const branchItems = branches.data?.items;
  // Header branch, else the only branch, else every branch.
  const branch =
    branchChoice ??
    (branchItems?.some((b) => b.uuid === selectedBranch)
      ? selectedBranch!
      : branchItems?.length === 1
        ? branchItems[0].uuid
        : ALL);

  const from = view === "day" ? date : weekStart(date);
  const to = view === "day" ? date : addDays(from, 6);
  const params = useMemo<CalendarParams | null>(() => {
    if (!branchItems) return null;
    return {
      from,
      to,
      by: branch === ALL ? "branch" : by,
      branch: branch === ALL ? undefined : branch,
      include_cancelled: includeCancelled ? "true" : undefined,
    };
  }, [branch, branchItems, by, from, includeCancelled, to]);

  const query = useReservationCalendar(params);
  const calendar = query.data;

  useChannel(
    organization?.uuid && can(permissions.tenant.reservations.read)
      ? tenantResourceChannel(organization.uuid, RESERVATIONS_RESOURCE)
      : null,
    () => void query.refetch(),
  );

  const step = view === "day" ? 1 : 7;
  const resetColumns = () => setColumnKey(ALL);

  const openItem = useCallback(
    (item: CalendarItem) =>
      router.push(
        routes.tenant.reservations.detail(slug, item.reservation_uuid),
      ),
    [router, slug],
  );

  const startBooking = useCallback(
    (point: GridPoint) => {
      const search = new URLSearchParams({
        date: point.date,
        at: clock(point.minutes),
      });
      const column = point.column;
      if (column?.kind === "branch") search.set("branch", column.key);
      else if (branch !== ALL) search.set("branch", branch);
      if (column?.kind === "staff") search.set("staff", column.key);
      router.push(
        `${routes.tenant.reservations.create(slug)}?${search.toString()}`,
      );
    },
    [branch, router, slug],
  );

  return (
    <EntityPage
      title={t("reservations.calendar.title")}
      description={t("reservations.calendar.description")}
      permission={READ_ANY}
      permissionMode="any"
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("reservations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("reservations.calendar.title") },
      ]}
      actions={
        canWrite ? (
          <EntityCreateButton
            onClick={() => router.push(routes.tenant.reservations.create(slug))}
            label={t("reservations.actions.create")}
          />
        ) : null
      }
    >
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex items-center gap-1">
            <Button
              type="button"
              variant="outline"
              size="icon"
              aria-label={t("reservations.calendar.previous")}
              onClick={() => setDate((d) => addDays(d, -step))}
            >
              <ChevronLeft className="size-4" />
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => setDate(dateKeyInZone(new Date(), zone))}
            >
              {t("reservations.calendar.today")}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="icon"
              aria-label={t("reservations.calendar.next")}
              onClick={() => setDate((d) => addDays(d, step))}
            >
              <ChevronRight className="size-4" />
            </Button>
          </div>
          <DatePicker
            className="w-44"
            value={date}
            onChange={(next) => next && setDate(next)}
          />
          <ToggleGroup
            type="single"
            variant="outline"
            value={view}
            onValueChange={(next) => next && setView(next as CalendarView)}
            aria-label={t("reservations.calendar.view")}
          >
            <ToggleGroupItem value="day">
              {t("reservations.calendar.day")}
            </ToggleGroupItem>
            <ToggleGroupItem value="week">
              {t("reservations.calendar.week")}
            </ToggleGroupItem>
          </ToggleGroup>
          <Select
            value={branch}
            onValueChange={(next) => {
              setBranchChoice(next);
              resetColumns();
            }}
          >
            <SelectTrigger
              className="w-48"
              aria-label={t("reservations.fields.branch")}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>
                {t("reservations.calendar.all_branches")}
              </SelectItem>
              {(branchItems ?? []).map((b) => (
                <SelectItem key={b.uuid} value={b.uuid}>
                  {b.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {branch !== ALL ? (
            <ToggleGroup
              type="single"
              variant="outline"
              value={by}
              onValueChange={(next) => {
                if (!next) return;
                setBy(next as "staff" | "resource");
                resetColumns();
              }}
              aria-label={t("reservations.calendar.columns")}
            >
              <ToggleGroupItem value="staff">
                {t("reservations.calendar.by_staff")}
              </ToggleGroupItem>
              <ToggleGroupItem value="resource">
                {t("reservations.calendar.by_resource")}
              </ToggleGroupItem>
            </ToggleGroup>
          ) : null}
          {view === "week" && calendar && calendar.columns.length > 1 ? (
            <Select value={columnKey} onValueChange={setColumnKey}>
              <SelectTrigger
                className="w-44"
                aria-label={t("reservations.calendar.columns")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>
                  {t("reservations.calendar.all_columns")}
                </SelectItem>
                {calendar.columns.map((c) => (
                  <SelectItem key={c.key} value={c.key}>
                    {c.kind === "unassigned"
                      ? t("reservations.calendar.unassigned")
                      : c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : null}
          <div className="flex items-center gap-2">
            <Switch
              id="calendar-cancelled"
              checked={includeCancelled}
              onCheckedChange={setIncludeCancelled}
            />
            <Label htmlFor="calendar-cancelled">
              {t("reservations.calendar.show_cancelled")}
            </Label>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="ms-auto"
            aria-label={t("reservations.calendar.refresh")}
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw
              className={query.isFetching ? "size-4 animate-spin" : "size-4"}
            />
          </Button>
        </div>

        <div className="flex flex-wrap gap-2">
          {LEGEND.map((status) => (
            <StatusChip
              key={status}
              label={t(STATUS[status].key)}
              tone={STATUS[status].tone}
            />
          ))}
        </div>

        {query.isLoading || branches.isLoading ? (
          <Loading label={t("common.loading")} />
        ) : null}
        {query.isError || branches.isError ? (
          <ErrorState
            title={t("reservations.calendar.load_failed")}
            onRetry={() => {
              void branches.refetch();
              void query.refetch();
            }}
            retryLabel={t("common.retry")}
          />
        ) : null}
        {calendar ? (
          calendar.columns.length === 0 ? (
            <EmptyState
              title={t("reservations.calendar.no_columns")}
              description={t(
                by === "resource" && branch !== ALL
                  ? "reservations.calendar.no_resources"
                  : "reservations.calendar.no_staff",
              )}
            />
          ) : (
            <CalendarGrid
              calendar={calendar}
              view={view}
              date={date}
              columnKey={columnKey}
              canWrite={canWrite}
              onItemClick={openItem}
              onEmptyClick={canWrite ? startBooking : undefined}
              onMove={
                canWrite
                  ? (item, point) => setMoveTarget({ item, point })
                  : undefined
              }
            />
          )
        ) : null}
      </div>

      <MoveDialog
        target={moveTarget}
        zone={calendar?.timezone ?? zone}
        onOpenChange={(open) => {
          if (!open) setMoveTarget(null);
        }}
      />
    </EntityPage>
  );
}
