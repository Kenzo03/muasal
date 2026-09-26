import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import ReactMarkdown from "react-markdown";
import rehypeSanitize from "rehype-sanitize";
import remarkGfm from "remark-gfm";
import { serverApi } from "@/lib/server-api";
import PrintButton from "./PrintButton";

// The print view (FSD §12.1): A4 with a header and page numbers; the browser
// saves it as PDF. The top bar is hidden when printing.
export default async function PrintSummaryPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const api = await serverApi();
  const { data } = await api.GET("/summaries/{id}", { params: { path: { id: Number(id) } } });
  if (!data) notFound();
  const t = await getTranslations("summaries");
  return (
    <main className="summary-print mx-auto max-w-[210mm] bg-white px-[16mm] py-[12mm] text-ink print:p-0">
      <div className="mb-4 flex items-center gap-3 border-b border-line pb-2 text-xs text-muted">
        <span className="font-semibold">Muasal · {data.project_key}</span>
        <span className="ml-auto">{data.title}</span>
        <PrintButton label={t("printNow")} />
      </div>
      <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>{data.markdown}</ReactMarkdown>
    </main>
  );
}
