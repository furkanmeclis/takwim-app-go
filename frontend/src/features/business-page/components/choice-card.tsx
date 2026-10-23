import type { ReactNode } from "react";

import {
  Field,
  FieldContent,
  FieldDescription,
  FieldLabel,
  FieldTitle,
} from "@/components/ui/field";
import { cn } from "@/lib/utils";

/**
 * A tappable option card (shadcn choice card): a radio or checkbox with a
 * title, a description and an aside such as a price. The whole card is the
 * label, so the touch target is large on phones.
 */
export function ChoiceCard({
  id,
  title,
  description,
  aside,
  control,
  disabled,
  className,
}: {
  id: string;
  title: ReactNode;
  description?: ReactNode;
  aside?: ReactNode;
  /** RadioGroupItem or Checkbox with the same `id`. */
  control: ReactNode;
  disabled?: boolean;
  className?: string;
}) {
  return (
    <FieldLabel htmlFor={id} className={cn("bg-card", className)}>
      <Field
        orientation="horizontal"
        data-disabled={disabled ? "true" : undefined}
      >
        {control}
        <FieldContent>
          <FieldTitle>{title}</FieldTitle>
          {description ? (
            <FieldDescription>{description}</FieldDescription>
          ) : null}
        </FieldContent>
        {aside ? (
          <div className="shrink-0 text-end text-sm font-medium tabular-nums">
            {aside}
          </div>
        ) : null}
      </Field>
    </FieldLabel>
  );
}
