import Link from "next/link";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import type { Chip, Turn } from "@/components/ask/Answer";
import PageBar from "@/components/PageBar";
import { getMe, getProject, serverApi } from "@/lib/server-api";
import { button, cx } from "@/lib/ui";
import AskPage from "./AskPage";

type Search = Record<string, string | string[] | undefined>;
const list = (v: string | string[] | undefined) => (Array.isArray(v) ? v : v ? [v] : []);

// /ask (FSD §10.1): the asker's threads on the side, the open thread's
// answers, and the Ask box. Links from other pages preset the chips with
// ?project=KEY&node=ID&client=ID, and the Home box passes ?q=.
export default async function Page({ searchParams }: { searchParams: Promise<Search> }) {
  const me = await getMe();
  if (!me) redirect("/login");
  const sp = await searchParams;
  const t = await getTranslations("ask");
  const api = await serverApi();
  const threadId = Number(list(sp.thread)[0]) || undefined;
  const [threads, detail] = await Promise.all([
    api.GET("/ask/threads"),
    threadId ? api.GET("/ask/threads/{id}", { params: { path: { id: threadId } } }) : Promise.resolve(undefined),
  ]);
  const history: Turn[] = (detail?.data?.queries ?? []).map((q) => ({
    question: q.question,
    streaming: false,
    evidence: q.evidence ?? [],
    claims: q.claims,
    status: q.status as Turn["status"],
    model: q.model,
    queryId: q.id,
    feedback: q.feedback,
    closest: [],
    results: [],
  }));

  // Preset chips, named through the API, which checks what the asker may see.
  const chips: Chip[] = [];
  const key = list(sp.project)[0];
  const project = key ? await getProject(key) : undefined;
  if (project) chips.push({ kind: "project", id: project.id, label: project.key });
  for (const id of list(sp.node).map(Number).filter(Number.isInteger)) {
    const { data } = await api.GET("/nodes/{id}", { params: { path: { id } } });
    if (data) chips.push({ kind: "node", id, label: [...data.path.map((p) => p.name), data.node.name].join(" › ") });
  }
  const clientId = Number(list(sp.client)[0]);
  if (project && clientId) {
    const { data } = await api.GET("/projects/{key}/clients", { params: { path: { key: project.key } } });
    const client = data?.items.find((c) => c.id === clientId);
    if (client) chips.push({ kind: "client", id: client.id, label: client.name });
  }

  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
      </PageBar>
      <div className="flex flex-col gap-4 p-4 md:flex-row md:p-5">
        <aside aria-label={t("threads")} className="flex w-full shrink-0 flex-col gap-2 md:w-64">
          <Link href="/ask" className={cx(button.secondary, "justify-start")}>{t("newThread")}</Link>
          {(threads.data?.items ?? []).length === 0 ? (
            <p className="text-[13px] text-muted">{t("noThreads")}</p>
          ) : (
            <ul className="flex flex-col text-[13px]">
              {threads.data?.items.map((th) => (
                <li key={th.id}>
                  <Link
                    href={`/ask?thread=${th.id}`}
                    aria-current={th.id === threadId ? "page" : undefined}
                    className={cx("block truncate rounded px-2 py-1.5 no-underline hover:bg-paper", th.id === threadId ? "bg-accent-soft font-semibold text-ink" : "text-ink")}
                  >
                    {th.title}
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </aside>
        <main className="min-w-0 flex-1">
          <AskPage key={threadId ?? "new"} threadId={detail?.data ? threadId : undefined} history={history} chips={chips} question={list(sp.q)[0]} />
        </main>
      </div>
    </>
  );
}
