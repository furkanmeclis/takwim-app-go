import {
  SOURCE_LABEL_KEYS,
  VERIFICATION_LABEL_KEYS,
} from "@/features/customers/constants";
import type { Customer } from "@/features/customers/types";

type Translate = (key: string) => string;

export function sourceLabel(t: Translate, source: string) {
  const key = SOURCE_LABEL_KEYS[source as Customer["source"]];
  return key ? t(key) : source;
}

export function verificationLabel(t: Translate, source: string) {
  const key = VERIFICATION_LABEL_KEYS[source];
  return key ? t(key) : source;
}
