import type {
  ReservationAnswer,
  ReservationFormField,
} from "@/features/reservations/types";

/** A booking question as the panel and the customer screens show it. */
export type FormQuestion = Pick<
  ReservationFormField,
  "key" | "label" | "type" | "options" | "required" | "help"
>;

/** Form values of the booking questions: text, number as text, checkbox. */
export type CustomFieldValues = Record<string, string | boolean>;

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Questions asked in the panel, in the form's order. */
export function panelFields(fields: ReservationFormField[] | undefined) {
  return (fields ?? []).filter((field) => field.panel);
}

/** Form values from stored answers (empty for a new booking). */
export function customDefaults(
  fields: FormQuestion[],
  stored: Record<string, unknown> = {},
): CustomFieldValues {
  const out: CustomFieldValues = {};
  for (const field of fields) {
    const value = stored[field.key];
    if (field.type === "checkbox") {
      out[field.key] = value === true;
    } else {
      out[field.key] =
        value === undefined || value === null ? "" : String(value);
    }
  }
  return out;
}

/**
 * Answers for the API: numbers as numbers, empty answers left out; a
 * checkbox is sent only when ticked or already stored. `stored` keeps the
 * answers of questions not asked in the panel (online-only, older ones).
 */
export function toCustomFields(
  fields: FormQuestion[],
  values: CustomFieldValues | undefined,
  stored: Record<string, unknown> = {},
): Record<string, unknown> {
  const out: Record<string, unknown> = { ...stored };
  for (const field of fields) {
    const value = values?.[field.key];
    delete out[field.key];
    if (field.type === "checkbox") {
      if (value === true || field.key in stored)
        out[field.key] = value === true;
      continue;
    }
    const text = typeof value === "string" ? value.trim() : "";
    if (text === "") continue;
    out[field.key] =
      field.type === "number" ? Number(text.replace(",", ".")) : text;
  }
  return out;
}

/** Client-side check of the answers (the API checks again). */
export function customFieldIssues(
  fields: FormQuestion[],
  values: CustomFieldValues | undefined,
  t: Translate,
): { key: string; message: string }[] {
  const issues: { key: string; message: string }[] = [];
  for (const field of fields) {
    const value = values?.[field.key];
    if (field.type === "checkbox") {
      if (field.required && value !== true) {
        issues.push({
          key: field.key,
          message: t("reservations.custom_fields.must_accept"),
        });
      }
      continue;
    }
    const text = typeof value === "string" ? value.trim() : "";
    if (text === "") {
      if (field.required) {
        issues.push({
          key: field.key,
          message: t("reservations.custom_fields.required"),
        });
      }
      continue;
    }
    if (
      field.type === "number" &&
      !Number.isFinite(Number(text.replace(",", ".")))
    ) {
      issues.push({
        key: field.key,
        message: t("reservations.custom_fields.number"),
      });
    }
    if (field.type === "text" && text.length > 500) {
      issues.push({
        key: field.key,
        message: t("reservations.custom_fields.too_long"),
      });
    }
  }
  return issues;
}

/** Display text of an answer ("Evet" / "Hayır" for a checkbox). */
export function answerText(
  answer: Pick<ReservationAnswer, "type" | "value">,
  t: Translate,
) {
  if (answer.type === "checkbox" || typeof answer.value === "boolean") {
    return answer.value === true
      ? t("reservations.custom_fields.yes")
      : t("reservations.custom_fields.no");
  }
  return String(answer.value ?? "");
}

/** "Saç tipi: Düz · Alerji: yok" for a list cell. */
export function answersSummary(answers: ReservationAnswer[], t: Translate) {
  return answers.map((a) => `${a.label}: ${answerText(a, t)}`).join(" · ");
}
