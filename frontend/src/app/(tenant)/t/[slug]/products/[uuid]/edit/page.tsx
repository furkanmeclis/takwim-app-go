import { ProductEditPage } from "@/features/products";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantProductEditPage({ params }: PageProps) {
  const { uuid } = await params;
  return <ProductEditPage uuid={uuid} />;
}
