import { SessionDetailPage } from "@/features/classes";

type PageProps = {
  params: Promise<{ slug: string; uuid: string; date: string }>;
};

export default async function TenantClassSessionPage({ params }: PageProps) {
  const { uuid, date } = await params;
  return <SessionDetailPage uuid={uuid} date={date} />;
}
