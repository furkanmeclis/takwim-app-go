import { ResourceDetailPage } from "@/features/resources";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantResourceDetailPage({ params }: PageProps) {
  const { uuid } = await params;
  return <ResourceDetailPage uuid={uuid} />;
}
