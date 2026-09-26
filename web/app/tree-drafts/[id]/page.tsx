import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { serverApi } from "@/lib/server-api";
import Review from "./Review";

export default async function TreeDraftPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const api = await serverApi();
  const { data } = await api.GET("/tree-drafts/{id}", { params: { path: { id: Number(id) } } });
  if (!data) notFound();
  const t = await getTranslations("treeDraft");
  const doc = await api.GET("/documents/{key}", { params: { path: { key: data.document_key } } });
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("heading")}</h1>
        <span className="text-[13px] text-muted">{data.document_key} · {data.document_title}</span>
      </PageBar>
      <main className="px-4 py-4 md:px-5">
        <Review initial={data} projectKey={doc.data?.project_key ?? ""} />
      </main>
    </>
  );
}
