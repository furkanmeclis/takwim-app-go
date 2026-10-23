"use client";

import { Minus, Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  MAX_PARTY,
  MIN_PARTY,
  validParty,
} from "@/features/business-page/lib/wizard";
import { useLocale } from "@/providers/locale-provider";

/** Party sizes offered as one tap; any other number is typed. */
const QUICK = [1, 2, 3, 4, 5, 6, 7, 8];

/**
 * Party size step (restaurant): a quick pick for small groups and a number
 * field with − / + for any size the API takes (1–1000). The value is the
 * typed text, so the field can be cleared while typing.
 */
export function PartyStep({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useLocale();
  const party = validParty(Number(value));
  const invalid = value !== "" && party === null;
  const step = (delta: number) => {
    const next = Math.min(
      MAX_PARTY,
      Math.max(MIN_PARTY, (party ?? MIN_PARTY - delta) + delta),
    );
    onChange(String(next));
  };

  return (
    <div className="space-y-5">
      <ToggleGroup
        type="single"
        variant="outline"
        spacing={2}
        value={party && QUICK.includes(party) ? String(party) : ""}
        onValueChange={(next) => {
          if (next) onChange(next);
        }}
        className="grid w-full grid-cols-4 gap-2"
        aria-label={t("business_page.steps.party_size")}
      >
        {QUICK.map((n) => (
          <ToggleGroupItem
            key={n}
            value={String(n)}
            className="h-12 w-full text-base tabular-nums"
          >
            {n}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>

      <Field data-invalid={invalid ? "true" : undefined}>
        <FieldLabel htmlFor="party-size">
          {t("business_page.party.other")}
        </FieldLabel>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="icon"
            onClick={() => step(-1)}
            disabled={party !== null && party <= MIN_PARTY}
            aria-label={t("business_page.party.decrease")}
          >
            <Minus className="size-4" />
          </Button>
          <Input
            id="party-size"
            type="number"
            inputMode="numeric"
            min={MIN_PARTY}
            max={MAX_PARTY}
            step={1}
            value={value}
            onChange={(event) =>
              onChange(event.target.value.replace(/\D/g, "").slice(0, 4))
            }
            aria-invalid={invalid}
            className="w-24 text-center tabular-nums"
          />
          <Button
            type="button"
            variant="outline"
            size="icon"
            onClick={() => step(1)}
            disabled={party !== null && party >= MAX_PARTY}
            aria-label={t("business_page.party.increase")}
          >
            <Plus className="size-4" />
          </Button>
          <span className="text-muted-foreground text-sm">
            {t("business_page.party.people")}
          </span>
        </div>
        <FieldDescription>
          {invalid
            ? t("business_page.party.invalid", {
                min: MIN_PARTY,
                max: MAX_PARTY,
              })
            : t("business_page.party.hint")}
        </FieldDescription>
      </Field>
    </div>
  );
}
