import { ServiceEditPage } from "@/features/services";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantServiceEditPage({ params }: PageProps) {
  const { uuid } = await params;
  return <ServiceEditPage uuid={uuid} />;
}
