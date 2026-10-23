import { ServiceDetailPage } from "@/features/services";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantServiceDetailPage({ params }: PageProps) {
  const { uuid } = await params;
  return <ServiceDetailPage uuid={uuid} />;
}
