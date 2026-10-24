import type { Metadata } from "next";

import { TestPaymentPage } from "@/features/business-page";

// Page of the development payment provider; the API answers 404 outside
// development and test.
export const metadata: Metadata = {
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};

export default async function Page({
  params,
}: {
  params: Promise<{ ref: string }>;
}) {
  const { ref } = await params;
  return <TestPaymentPage paymentRef={ref} />;
}
