import Link from "next/link";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import type { Chip, Turn } from "@/components/ask/Answer";
import Icon from "@/components/Icon";
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
  const question = list(sp.q)[0];
  const [threads, detail, updates] = await Promise.all([
    api.GET("/ask/threads"),
    threadId ? api.GET("/ask/threads/{id}", { params: { path: { id: threadId } } }) : Promise.resolve(undefined),
    threadId || question ? Promise.resolve(undefined) : api.GET("/me/updates"),
  ]);
  // A new thread offers examples, one about the ticket that changed last (MSL-37).
  const latest = updates?.data?.items[0];
  const examples = updates ? [t("exampleChanged"), ...(latest ? [t("exampleTicket", { key: latest.key })] : []), t("exampleDecisions")] : [];
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
        <aside aria-label={t("threads")} className="flex w-full shrink-0 flex-col gap-3 md:w-64">
          <Link href="/ask" className={button.secondary}>
            <Icon name="plus" className="size-4 text-accent" />
            {t("newThread")}
          </Link>
          {(threads.data?.items ?? []).length === 0 ? (
            <p className="text-[13px] text-muted">{t("noThreads")}</p>
          ) : (
            <ul className="flex flex-col gap-0.5 text-[13.5px]">
              {threads.data?.items.map((th) => (
                <li key={th.id}>
                  <Link
                    href={`/ask?thread=${th.id}`}
                    aria-current={th.id === threadId ? "page" : undefined}
                    className={cx(
                      "block truncate rounded-xl px-3 py-2.5 no-underline",
                      th.id === threadId
                        ? "bg-white font-bold text-ink shadow-[0_1px_2px_rgba(43,36,32,0.08),0_0_0_1px_rgba(43,36,32,0.05)] hover:text-ink"
                        : "font-medium text-ink-soft hover:bg-white/70 hover:text-ink",
                    )}
                  >
                    {th.title}
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </aside>
        <main className="min-w-0 flex-1">
          <AskPage key={threadId ?? "new"} threadId={detail?.data ? threadId : undefined} history={history} chips={chips} question={question} examples={examples} />
        </main>
      </div>
    </>
  );
}
