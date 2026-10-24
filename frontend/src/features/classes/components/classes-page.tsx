"use client";

import { Plus } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useBusinessToday } from "@/features/cash/lib/format";
import { SessionsTable } from "@/features/classes/components/sessions-table";
import { TimetableTable } from "@/features/classes/components/timetable-table";
import { MAX_SESSION_DAYS } from "@/features/classes/constants";
import { addDays } from "@/features/classes/lib/filters";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Tab = "sessions" | "timetable";
type Range = { from: string; to: string };

/**
 * The days of the sessions list. The end never comes before the start
 * and the range stays within the API's limit (31 days).
 */
function SessionRange({
  value,
  onChange,
}: {
  value: Range;
  onChange: (value: Range) => void;
}) {
  const { t } = useLocale();
  const today = useBusinessToday();
  const last = (from: string) => addDays(from, MAX_SESSION_DAYS - 1);

  return (
    <div className="flex w-full flex-wrap items-center gap-2 sm:w-auto">
      <DatePicker
        className="w-full sm:w-40"
        value={value.from}
        onChange={(next) => {
          const from = next || today;
          let to = value.to < from ? from : value.to;
          if (to > last(from)) to = last(from);
          onChange({ from, to });
        }}
        placeholder={t("classes.sessions.from")}
      />
      <DatePicker
        className="w-full sm:w-40"
        value={value.to}
        onChange={(next) => {
          const to = next || addDays(value.from, 6);
          let from = value.from > to ? to : value.from;
          if (to > last(from)) from = addDays(to, -(MAX_SESSION_DAYS - 1));
          onChange({ from, to });
        }}
        placeholder={t("classes.sessions.to")}
      />
    </div>
  );
}

/**
 * Group class timetable (TEC-970, gym `classes` module): the sessions of
 * the coming days with their seats and the weekly classes they come
 * from. A group class can be booked only at its sessions.
 */
export function ClassesPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const today = useBusinessToday();
  const [tab, setTab] = useState<Tab>("sessions");
  const [creating, setCreating] = useState(false);
  const [range, setRange] = useState<Range>(() => ({
    from: today,
    to: addDays(today, 6),
  }));
  const canWrite = can(permissions.tenant.classes.write);

  return (
    <EntityPage
      title={t("classes.title")}
      description={t("classes.description")}
      permission={permissions.tenant.classes.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("classes.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("classes.title") },
      ]}
      actions={
        canWrite ? (
          <Button
            onClick={() => {
              setTab("timetable");
              setCreating(true);
            }}
          >
            <Plus className="size-4" />
            {t("classes.actions.create")}
          </Button>
        ) : null
      }
    >
      <Tabs
        value={tab}
        onValueChange={(value) => setTab(value as Tab)}
        className="space-y-4"
      >
        <TabsList>
          <TabsTrigger value="sessions">
            {t("classes.tabs.sessions")}
          </TabsTrigger>
          <TabsTrigger value="timetable">
            {t("classes.tabs.timetable")}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="sessions">
          {tab === "sessions" ? (
            <EntitySectionCard
              title={t("classes.tabs.sessions")}
              actions={<SessionRange value={range} onChange={setRange} />}
            >
              <p className="text-muted-foreground mb-4 text-sm">
                {t("classes.sessions.hint")}
              </p>
              <SessionsTable canWrite={canWrite} range={range} />
            </EntitySectionCard>
          ) : null}
        </TabsContent>

        <TabsContent value="timetable">
          {tab === "timetable" ? (
            <EntitySectionCard title={t("classes.tabs.timetable")}>
              <p className="text-muted-foreground mb-4 text-sm">
                {t("classes.timetable_hint")}
              </p>
              <TimetableTable
                canWrite={canWrite}
                creating={creating}
                onCreatingChange={setCreating}
              />
            </EntitySectionCard>
          ) : null}
        </TabsContent>
      </Tabs>
    </EntityPage>
  );
}
