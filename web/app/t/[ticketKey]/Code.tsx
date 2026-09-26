import { useLocale, useTranslations } from "next-intl";
import type { components } from "@/lib/api-types";
import { utc } from "@/lib/format";
import { chip, cx, panel, sectionTitle } from "@/lib/ui";

type TicketCode = components["schemas"]["TicketCode"];

const stateStyle: Record<string, string> = {
  merged: "bg-violet-100 text-violet-800",
  open: "bg-green-100 text-green-800",
  opened: "bg-green-100 text-green-800",
  closed: "bg-gray-100 text-gray-700",
};

// Code (FSD §14.1): merge requests and commits whose text names this ticket,
// as the Git webhooks reported them.
export default function Code({ code }: { code: TicketCode }) {
  const t = useTranslations("code");
  const locale = useLocale();
  if (code.merge_requests.length === 0 && code.commits.length === 0) return null;
  return (
    <section aria-labelledby="code-title" className={cx(panel, "flex flex-col gap-2.5 px-4 py-3.5")}>
      <h2 id="code-title" className={sectionTitle}>{t("title")}</h2>
      {code.merge_requests.length > 0 && (
        <ul aria-label={t("mergeRequests")} className="flex flex-col gap-1.5">
          {code.merge_requests.map((m) => {
            const state = m.state === "opened" ? "open" : m.state;
            return (
              <li key={`${m.repo}!${m.number}`} className="flex flex-wrap items-center gap-2 text-[13px]">
                <span className={cx(chip, stateStyle[state] ?? stateStyle.closed)}>{t.has(`state.${state}`) ? t(`state.${state}`) : state}</span>
                <span className="font-mono text-xs text-muted">{m.repo}#{m.number}</span>
                {m.url ? <a href={m.url} target="_blank" rel="noreferrer" className="min-w-0 truncate">{m.title}</a> : <span className="min-w-0 truncate">{m.title}</span>}
                {m.merged_at && <span className="ml-auto text-xs text-muted">{utc(m.merged_at, locale)}</span>}
              </li>
            );
          })}
        </ul>
      )}
      {code.commits.length > 0 && (
        <ul aria-label={t("commits")} className="flex flex-col gap-1.5">
          {code.commits.map((c) => (
            <li key={`${c.repo}@${c.sha}`} className="flex flex-wrap items-center gap-2 text-[13px]">
              {c.url ? (
                <a href={c.url} target="_blank" rel="noreferrer" className="font-mono text-xs">{c.sha.slice(0, 7)}</a>
              ) : (
                <span className="font-mono text-xs">{c.sha.slice(0, 7)}</span>
              )}
              <span className="min-w-0 flex-1 truncate">{c.message}</span>
              <span className="text-xs text-muted">
                {[c.author, c.committed_at && utc(c.committed_at, locale)].filter(Boolean).join(" · ")}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
