import Link from "next/link";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { ProjectOf } from "@/app/Frame";
import { DraftStatusChip } from "@/components/Chips";
import Icon from "@/components/Icon";
import PageBar from "@/components/PageBar";
import { serverApi } from "@/lib/server-api";
import Review from "./Review";

export default async function TreeDraftPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const api = await serverApi();
  const { data } = await api.GET("/tree-drafts/{id}", { params: { path: { id: Number(id) } } });
  if (!data) notFound();
  const t = await getTranslations("treeDraft");
  const td = await getTranslations("documents");
  const tt = await getTranslations("ticket");
  const doc = await api.GET("/documents/{key}", { params: { path: { key: data.document_key } } });
  const projectKey = doc.data?.project_key ?? "";
  return (
    <>
      {projectKey && <ProjectOf projectKey={projectKey} />}
      <PageBar>
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <nav aria-label={tt("path")} className="flex flex-wrap items-center gap-1.5 text-[13px] text-muted">
            {projectKey && (
              <>
                <Link href={`/p/${projectKey}/documents`}>{td("heading")}</Link>
                <Icon name="chevronRight" className="size-3.5" />
              </>
            )}
            <Link href={`/documents/${data.document_key}`} className="min-w-0 truncate">{data.document_key} · {data.document_title}</Link>
          </nav>
          <div className="flex flex-wrap items-center gap-2.5">
            <h1>{t("heading")}</h1>
            <DraftStatusChip status={data.status} label={td(`status.${data.status}`)} />
          </div>
        </div>
      </PageBar>
      <main className="px-4 py-4 md:px-5">
        <Review initial={data} projectKey={projectKey} />
      </main>
    </>
  );
}
