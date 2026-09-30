import { notFound, redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { getMe, serverApi } from "@/lib/server-api";
import ImportDetail from "./ImportDetail";

export default async function ImportPage({ params }: { params: Promise<{ id: string }> }) {
  const me = await getMe();
  if (!me) redirect("/login"); // the API shows an import only to those who may run it (MSL-49)
  const id = Number((await params).id);
  const { data } = await (await serverApi()).GET("/imports/{id}", { params: { path: { id } } });
  if (!data) notFound();
  const t = await getTranslations("imports");
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("detailTitle", { file: data.file_name })}</h1>
        <span className="font-mono text-[13px] text-muted">{data.project_key}</span>
      </PageBar>
      <main className="flex max-w-5xl flex-col gap-4 p-4 md:p-5">
        <ImportDetail initial={data} />
      </main>
    </>
  );
}
