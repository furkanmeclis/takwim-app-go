import { AcceptInvitationPage } from "@/features/invitations/components/accept-invitation-page";

export default async function InvitePage({
  params,
}: {
  params: Promise<{ token: string }>;
}) {
  const { token } = await params;
  return <AcceptInvitationPage token={token} />;
}
