"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { askStream, type AskIgnore, type AskRequest, type AskScope, type AskScopeEvent } from "@/lib/ask";
import Icon from "@/components/Icon";
import { button, cx, field } from "@/lib/ui";
import Answer, { ChipView, type Chip, type Turn } from "./Answer";

type Props = {
  chips?: Chip[]; // preset by the page: project, node, client
  threadId?: number;
  question?: string; // asked at once, e.g. from the Home Ask box
  onThread?: (id: number) => void;
  compact?: boolean;
};

// The Ask box and the answers of one thread (FSD §10). The page's chips stay
// for the whole thread until the user removes one; each question streams its
// answer claim by claim.
export default function AskView({ chips: preset = [], threadId: initialThread, question: initial, onThread, compact }: Props) {
  const t = useTranslations("ask");
  const [chips, setChips] = useState<Chip[]>(preset);
  const [language, setLanguage] = useState<"auto" | "id" | "en">("auto");
  const [question, setQuestion] = useState("");
  const [turns, setTurns] = useState<Turn[]>([]);
  const [threadId, setThreadId] = useState(initialThread);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const busy = turns.some((x) => x.streaming);
  const started = useRef(false);
  const ignoredBy = useRef(new Map<number, AskIgnore[]>()); // per turn: the detected chips removed
  const thread = useRef<HTMLDivElement>(null);

  // In the panel the thread scrolls above a pinned Ask box, so a new question
  // would start out of sight: bring it to the top of the thread.
  useEffect(() => {
    if (compact && turns.length > 0) thread.current?.lastElementChild?.scrollIntoView({ block: "start", behavior: "smooth" });
  }, [compact, turns.length]);

  async function run(q: string, ignore: AskIgnore[] = []) {
    const index = turns.length;
    const update = (f: (x: Turn) => Turn) => setTurns((xs) => xs.map((x, i) => (i === index ? f(x) : x)));
    setTurns((xs) => [...xs, { question: q, streaming: true, evidence: [], claims: [], closest: [], results: [] }]);
    const body: AskRequest = { question: q, language, scope: toScope(chips), thread_id: threadId, ignore: ignore.length > 0 ? ignore : undefined };
    ignoredBy.current.set(index, ignore);
    try {
      const refused = await askStream(body, (e) => {
        if (e.type === "queued") update((x) => ({ ...x, queued: e.position }));
        else if (e.type === "scope") update((x) => ({ ...x, queued: undefined, scope: scopeChips(chips, e.scope) }));
        else if (e.type === "evidence") update((x) => ({ ...x, evidence: e.items }));
        else if (e.type === "claim") update((x) => ({ ...x, claims: [...x.claims, e.claim] }));
        else if (e.type === "error") update((x) => ({ ...x, error: e.code }));
        else if (e.type === "result") {
          const r = e.result;
          update((x) => ({ ...x, streaming: false, status: r.status, model: r.model, closest: r.closest, results: r.results, queryId: r.query_id }));
          if (!threadId) {
            setThreadId(r.thread_id);
            onThread?.(r.thread_id);
          }
        }
      });
      if (refused) update((x) => ({ ...x, streaming: false, error: refused }));
    } catch {
      update((x) => ({ ...x, streaming: false, error: x.error ?? "internal" }));
    }
  }

  useEffect(() => {
    if (initial && !started.current) {
      started.current = true;
      run(initial);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initial]);

  // Removing a detected chip re-runs that question without it (§10.2).
  function removeDetected(index: number, c: Chip) {
    const before = ignoredBy.current.get(index) ?? [];
    const next: AskIgnore = c.kind === "date" ? { kind: "date" } : { kind: c.kind as AskIgnore["kind"], id: c.id };
    run(turns[index].question, [...before, next]);
  }

  function addDates() {
    if (!from && !to) return;
    setChips((cs) => [...cs.filter((c) => c.kind !== "date"), { kind: "date", label: "", from: from || undefined, to: to || undefined }]);
    setFrom("");
    setTo("");
  }

  return (
    <div className={compact ? "flex h-full flex-col" : "flex max-w-3xl flex-col gap-4"}>
      {(compact || turns.length > 0) && (
        <div ref={thread} className={cx("flex flex-col gap-4", compact && "min-h-0 flex-1 overflow-y-auto p-4")}>
          {turns.map((turn, i) => (
            // In the panel the newest turn is at least as tall as the thread, so
            // its question can scroll to the top while the answer streams below.
            <div key={i} className={compact && i === turns.length - 1 ? "min-h-full scroll-mt-4" : undefined}>
              <Answer turn={turn} onRemoveChip={i === turns.length - 1 && !busy ? (c) => removeDetected(i, c) : undefined} />
            </div>
          ))}
        </div>
      )}
      <form
        aria-label={t("form")}
        onSubmit={(e) => {
          e.preventDefault();
          const q = question.trim();
          if (!q || busy) return;
          setQuestion("");
          run(q);
        }}
        className={cx(
          "flex flex-col gap-2.5 rounded-2xl border border-field bg-white p-3.5 shadow-[0_1px_2px_rgba(43,36,32,0.04),0_8px_24px_rgba(43,36,32,0.06)] focus-within:border-accent",
          compact && "m-4 mt-0 shrink-0",
        )}
      >
        {chips.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {chips.map((c) => (
              <ChipView key={`${c.kind}:${c.id ?? c.from}`} chip={c} onRemove={(x) => setChips((cs) => cs.filter((y) => y !== x))} />
            ))}
          </div>
        )}
        <textarea
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              e.currentTarget.form?.requestSubmit();
            }
          }}
          aria-label={t("question")}
          placeholder={t("placeholder")}
          maxLength={1000}
          rows={compact ? 2 : 3}
          className="w-full resize-y bg-transparent px-1 py-1 text-[15px] leading-relaxed text-ink outline-none placeholder:text-muted"
        />
        <div className="flex flex-wrap items-center gap-2">
          <details className="relative">
            <summary className={cx(button.secondary, "h-8 list-none px-3 text-[13px]")}>
              <Icon name="calendar" className="size-3.5 text-muted" />
              {t("addDates")}
            </summary>
            {/* In the panel the box sits at the bottom, so the dates open upward. */}
            <div className={cx("absolute left-0 z-10 flex flex-col gap-2.5 rounded-xl border border-line bg-white p-3.5 shadow-[0_12px_32px_rgba(43,36,32,0.12),0_2px_6px_rgba(43,36,32,0.06)]", compact ? "bottom-full mb-1.5" : "mt-1.5")}>
              <label className={field.label}>
                {t("from")}
                <input type="date" value={from} onChange={(e) => setFrom(e.target.value)} className={field.compact} />
              </label>
              <label className={field.label}>
                {t("to")}
                <input type="date" value={to} onChange={(e) => setTo(e.target.value)} className={field.compact} />
              </label>
              <button
                type="button"
                className={button.secondary}
                onClick={(e) => {
                  addDates();
                  e.currentTarget.closest("details")?.removeAttribute("open");
                }}
              >
                {t("apply")}
              </button>
            </div>
          </details>
          <label className="flex h-8 items-center gap-1 rounded-[9px] border border-line bg-paper pl-3 text-[13px] font-semibold text-ink focus-within:border-accent">
            {t("language")}
            <select value={language} onChange={(e) => setLanguage(e.target.value as typeof language)} className="h-full cursor-pointer bg-transparent font-medium text-muted outline-none">
              <option value="auto">{t("langAuto")}</option>
              <option value="id">Bahasa Indonesia</option>
              <option value="en">English</option>
            </select>
          </label>
          <button type="submit" disabled={busy || question.trim() === ""} className={cx(button.primary, "ml-auto")}>
            {t("send")}
          </button>
        </div>
      </form>
    </div>
  );
}

function toScope(chips: Chip[]): AskScope {
  const ids = (kind: Chip["kind"]) => {
    const out = chips.filter((c) => c.kind === kind && c.id !== undefined).map((c) => c.id as number);
    return out.length > 0 ? out : undefined;
  };
  const date = chips.find((c) => c.kind === "date");
  return {
    project_ids: ids("project"),
    node_ids: ids("node"),
    client_ids: ids("client"),
    user_ids: ids("user"),
    contact_ids: ids("contact"),
    from: date?.from,
    to: date?.to,
  };
}

// The final chips: the explicit ones as sent, then the detected ones with
// their labels (§10.2, "Scope used" in §10.3).
function scopeChips(explicit: Chip[], scope: AskScopeEvent): Chip[] {
  const d = scope.detected;
  const detected: Chip[] = (d.labels ?? []).map((l) => ({ kind: l.kind, id: l.id, label: l.label, detected: true }));
  if (d.from || d.to) detected.push({ kind: "date", label: "", from: d.from, to: d.to, detected: true });
  return [...explicit, ...detected];
}
