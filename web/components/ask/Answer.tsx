"use client";

import { useState } from "react";
import Link from "next/link";
import { useLocale, useTranslations } from "next-intl";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import { answerMarkdown, itemHref, type AskClaim, type AskFeedback, type AskItem, type AskStatus } from "@/lib/ask";
import { day } from "@/lib/format";
import { button, chip, cx, field, sectionTitle } from "@/lib/ui";

export type Chip = {
  kind: "project" | "node" | "client" | "user" | "contact" | "date";
  id?: number;
  label: string;
  from?: string;
  to?: string;
  detected?: boolean;
};

export type Turn = {
  question: string;
  streaming: boolean;
  queued?: number;
  scope?: Chip[];
  evidence: AskItem[];
  claims: AskClaim[];
  status?: AskStatus;
  model?: string | null;
  closest: AskItem[];
  results: AskItem[];
  error?: string; // an AI error code (ai_busy, …) or a refused request's problem code
  queryId?: number; // for feedback, once the answer is logged
  feedback?: AskFeedback;
};

// One question and its answer, laid out as §10.3 lists: status line, claims
// with citation chips, sources, what was retrieved but not cited, the scope
// used and the model badge. It serves the live stream and the saved thread.
export default function Answer({ turn, onRemoveChip }: { turn: Turn; onRemoveChip?: (c: Chip) => void }) {
  const t = useTranslations("ask");
  const te = useTranslations("errors");
  const byKey = new Map(turn.evidence.map((it) => [it.key, it]));
  const cited = [...new Set(turn.claims.flatMap((c) => c.cites))].map((k) => byKey.get(k)).filter((it): it is AskItem => Boolean(it));
  const uncited = turn.evidence.filter((it) => !cited.includes(it));

  let status: React.ReactNode = null;
  if (turn.error) {
    const key = `errors.${turn.error}`;
    status = <p role="alert" className="text-[13px] text-danger">{t.has(key) ? t(key) : te.has(turn.error) ? te(turn.error) : t("errors.internal")}</p>;
  } else if (turn.streaming) {
    status = (
      <p role="status" className="flex items-center gap-2 text-[13px] text-muted">
        <span className="size-2 animate-pulse rounded-full bg-accent" />
        {turn.queued ? t("waiting", { count: turn.queued }) : turn.claims.length > 0 ? t("answering") : t("searching")}
      </p>
    );
  } else if (turn.status === "answered") {
    status = <p className="text-[13px] font-semibold text-ink">{t("answeredFrom", { count: turn.evidence.length })}</p>;
  } else if (turn.status === "not_enough_info") {
    status = <p className="text-[13px] font-semibold text-ink">{t("notEnough")}</p>;
  } else if (turn.status === "ai_off") {
    status = (
      <p className="text-[13px] text-ink">
        <span className="font-semibold">{t("aiOff")}</span> <span className="text-muted">{t("aiOffHint")}</span>
      </p>
    );
  }

  return (
    <article aria-label={turn.question} className="flex flex-col gap-3 border-b border-line-soft pb-4 last:border-0">
      <h3 className="text-[15px] font-semibold leading-snug">{turn.question}</h3>
      {status}
      {turn.claims.length > 0 && (
        <ul className="flex flex-col gap-2 text-sm leading-relaxed">
          {turn.claims.map((c, i) => (
            <li key={i} className="flex flex-wrap items-baseline gap-x-1.5">
              <span>{c.text}</span>
              {c.cites.map((k) => (
                <Cite key={k} itemKey={k} item={byKey.get(k)} />
              ))}
            </li>
          ))}
        </ul>
      )}
      {turn.status === "not_enough_info" && !turn.streaming && (
        <div className="flex flex-col gap-2">
          <p className="text-[13px] text-muted">{t("suggestions")}</p>
          {turn.closest.length > 0 && <ItemList title={t("closest")} items={turn.closest} />}
        </div>
      )}
      {turn.status === "ai_off" && !turn.streaming && (turn.results.length > 0 ? <ItemList title={t("results")} items={turn.results} /> : <p className="text-[13px] text-muted">{t("noResults")}</p>)}
      {cited.length > 0 && <ItemList title={t("sources")} items={cited} />}
      {uncited.length > 0 && !turn.streaming && turn.status === "answered" && (
        <details className="text-[13px]">
          <summary className="cursor-pointer text-muted">{t("alsoRetrieved", { count: uncited.length })}</summary>
          <ItemList items={uncited} />
        </details>
      )}
      {!turn.streaming && (
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted">
          {turn.scope && turn.scope.length > 0 && <span className={sectionTitle}>{t("scopeUsed")}</span>}
          {turn.scope?.map((c) => (
            <ChipView key={`${c.kind}:${c.id ?? c.from}`} chip={c} onRemove={c.detected ? onRemoveChip : undefined} />
          ))}
          {turn.model && <span className={cx(chip, "ml-auto bg-well text-[#4A423C]")}>{turn.model}</span>}
          {turn.claims.length > 0 && <CopyButton question={turn.question} claims={turn.claims} />}
        </div>
      )}
      {!turn.streaming && turn.queryId && (turn.status === "answered" || turn.status === "not_enough_info") && (
        <Feedback queryId={turn.queryId} initial={turn.feedback} />
      )}
    </article>
  );
}

// A citation chip: hovering or focusing shows the ticket's title, client,
// requester and date; a click opens it in a new tab (§10.3).
function Cite({ itemKey, item }: { itemKey: string; item?: AskItem }) {
  const t = useTranslations("ask");
  const locale = useLocale();
  return (
    <span className="group relative inline-flex">
      <Link
        href={itemHref(itemKey)}
        target="_blank"
        rel="noopener noreferrer"
        className={cx(chip, "bg-accent-soft font-mono text-accent-strong no-underline hover:underline")}
        aria-describedby={item ? `cite-${itemKey}` : undefined}
      >
        {itemKey}
      </Link>
      {item && (
        <span
          id={`cite-${itemKey}`}
          role="tooltip"
          className="pointer-events-none invisible absolute bottom-full left-0 z-20 mb-1 w-64 rounded border border-line bg-white p-2.5 text-xs leading-snug text-ink opacity-0 shadow-lg group-focus-within:visible group-focus-within:opacity-100 group-hover:visible group-hover:opacity-100"
        >
          <span className="block font-semibold">{item.title}</span>
          <span className="block text-muted">{item.client ?? t("core")}</span>
          {item.kind === "note" ? (
            <span className="block text-muted">{t("noteBy", { name: item.requested_by, date: day(item.date, locale) })}</span>
          ) : (
            <>
              <span className="block text-muted">{t("requestedBy", { name: item.requested_by })}</span>
              <span className="block text-muted">{t(item.closed ? "closedOn" : "createdOn", { date: day(item.date, locale) })}</span>
            </>
          )}
        </span>
      )}
    </span>
  );
}

function ItemList({ title, items }: { title?: string; items: AskItem[] }) {
  const t = useTranslations("ask");
  const locale = useLocale();
  return (
    <div className="flex flex-col gap-1.5">
      {title && <h4 className={sectionTitle}>{title}</h4>}
      <ul className="flex flex-col divide-y divide-line-soft rounded border border-line-soft text-[13px]">
        {items.map((it) => (
          <li key={it.key} className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 px-2.5 py-1.5">
            <Link href={itemHref(it.key)} className="font-mono text-xs font-semibold">{it.key}</Link>
            <span className="min-w-0 flex-1 font-medium">{it.title}</span>
            <span className="text-muted">{it.client ?? t("core")}</span>
            <span className="text-muted">{it.requested_by}</span>
            <span className="text-muted">{day(it.date, locale)}</span>
            <span className="text-muted">{it.kind === "note" ? t("noteKind") : it.status}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

// A scope chip. Detected ones have a dotted outline and a "detected" label,
// and removing one re-runs the question without it (§10.2).
export function ChipView({ chip: c, onRemove }: { chip: Chip; onRemove?: (c: Chip) => void }) {
  const t = useTranslations("ask");
  const locale = useLocale();
  const label = c.kind === "date" ? t("dates", { from: c.from ? day(c.from, locale) : "…", to: c.to ? day(c.to, locale) : "…" }) : c.label;
  return (
    <span
      className={cx(
        "inline-flex h-6 items-center gap-1 rounded-full px-2 text-xs text-ink",
        c.detected ? "border border-dashed border-muted bg-white" : "border border-line bg-paper",
      )}
    >
      <span className="text-muted">{t(`kinds.${c.kind}`)}</span>
      <span className="font-medium">{label}</span>
      {c.detected && <span className="text-[11px] italic text-muted">{t("detected")}</span>}
      {onRemove && (
        <button type="button" onClick={() => onRemove(c)} aria-label={t("removeChip", { label })} className="-mr-1 cursor-pointer rounded-full p-0.5 text-muted hover:bg-well hover:text-ink">
          <Icon name="x" className="size-3" />
        </button>
      )}
    </span>
  );
}

function CopyButton({ question, claims }: { question: string; claims: AskClaim[] }) {
  const t = useTranslations("ask");
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className={cx(button.quiet, "text-xs")}
      onClick={async () => {
        await navigator.clipboard.writeText(answerMarkdown(question, claims, window.location.origin));
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      }}
    >
      {copied ? t("copied") : t("copy")}
    </button>
  );
}

const reasons = ["wrong", "missing_tickets", "wrong_citation", "too_vague"] as const;

// Thumbs up or down on an answer (FSD §10.7). Down asks why, with reasons and
// free text; the Ask log shows it to system admins.
function Feedback({ queryId, initial }: { queryId: number; initial?: AskFeedback }) {
  const t = useTranslations("ask.feedback");
  const [rating, setRating] = useState(initial?.rating);
  const [asking, setAsking] = useState(false);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState(false);

  async function send(body: AskFeedback) {
    const { error } = await api.POST("/ask/queries/{id}/feedback", { params: { path: { id: queryId } }, body });
    setError(Boolean(error));
    if (error) return;
    setRating(body.rating);
    setSent(true);
    setAsking(false);
  }

  const thumb = (up: boolean) => cx(button.secondary, "h-7 px-2", rating === (up ? "up" : "down") && "border-accent bg-accent-soft");
  return (
    <div className="flex flex-col gap-2 text-xs">
      <div className="flex items-center gap-2 text-muted">
        <span>{t("question")}</span>
        <button type="button" aria-pressed={rating === "up"} onClick={() => send({ rating: "up" })} className={thumb(true)}>👍 {t("up")}</button>
        <button type="button" aria-pressed={rating === "down"} onClick={() => setAsking(true)} className={thumb(false)}>👎 {t("down")}</button>
        {sent && <span role="status">{t("thanks")}</span>}
        {error && <span role="alert" className="text-danger">{t("failed")}</span>}
      </div>
      {asking && (
        <form
          aria-label={t("why")}
          className="flex flex-col gap-2 rounded border border-line bg-paper p-2.5"
          onSubmit={(e) => {
            e.preventDefault();
            const form = new FormData(e.currentTarget);
            const comment = String(form.get("comment") ?? "").trim();
            send({ rating: "down", reasons: form.getAll("reason").map(String) as AskFeedback["reasons"], comment: comment || undefined });
          }}
        >
          <fieldset className="flex flex-wrap gap-3">
            <legend className="mb-1 font-medium text-ink">{t("why")}</legend>
            {reasons.map((r) => (
              <label key={r} className="flex items-center gap-1.5">
                <input type="checkbox" name="reason" value={r} defaultChecked={initial?.reasons?.includes(r)} className="accent-accent" />
                {t(`reasons.${r}`)}
              </label>
            ))}
          </fieldset>
          <textarea name="comment" maxLength={1000} rows={2} placeholder={t("comment")} aria-label={t("comment")} defaultValue={initial?.comment ?? ""} className={field.textarea} />
          <div className="flex gap-2">
            <button className={button.primary}>{t("send")}</button>
            <button type="button" onClick={() => setAsking(false)} className={button.secondary}>{t("cancel")}</button>
          </div>
        </form>
      )}
    </div>
  );
}
