import { ReservationDetailPage } from "@/features/reservations";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantReservationDetailPage({
  params,
}: PageProps) {
  const { uuid } = await params;
  return <ReservationDetailPage uuid={uuid} />;
}
