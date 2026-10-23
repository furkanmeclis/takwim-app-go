import type { Metadata } from "next";

import { QRLoginFallback } from "@/features/auth/components/qr-login-fallback";

// The session id is a short-lived secret: never leak it via Referer or index it.
export const metadata: Metadata = {
  referrer: "no-referrer",
  robots: { index: false, follow: false },
};

export default async function QRLoginFallbackPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <QRLoginFallback sessionId={id} />;
}
