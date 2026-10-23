import { ResourceEditPage } from "@/features/resources";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantResourceEditPage({ params }: PageProps) {
  const { uuid } = await params;
  return <ResourceEditPage uuid={uuid} />;
}
