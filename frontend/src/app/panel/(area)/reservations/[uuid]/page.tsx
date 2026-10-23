import { CustomerReservationDetailPage } from "@/features/customer-panel/components/customer-reservation-detail-page";

export default async function CustomerReservationRoute({
  params,
}: {
  params: Promise<{ uuid: string }>;
}) {
  const { uuid } = await params;
  return <CustomerReservationDetailPage uuid={uuid} />;
}
