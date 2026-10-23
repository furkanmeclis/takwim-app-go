import { ProductDetailPage } from "@/features/products";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantProductDetailPage({ params }: PageProps) {
  const { uuid } = await params;
  return <ProductDetailPage uuid={uuid} />;
}
