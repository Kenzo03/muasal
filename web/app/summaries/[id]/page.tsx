import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { ProjectOf } from "@/app/Frame";
import PageBar from "@/components/PageBar";
import { serverApi } from "@/lib/server-api";
import Editor from "./Editor";

export default async function SummaryPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const api = await serverApi();
  const { data } = await api.GET("/summaries/{id}", { params: { path: { id: Number(id) } } });
  if (!data) notFound();
  const t = await getTranslations("summaries");
  return (
    <>
      <ProjectOf projectKey={data.project_key} />
      <PageBar>
        <h1 className="text-base font-semibold">{t("summary")}</h1>
        <span className="font-mono text-[13px] text-muted">{data.project_key}</span>
      </PageBar>
      <main className="px-4 py-4 md:px-5">
        <Editor summary={data} />
      </main>
    </>
  );
}
