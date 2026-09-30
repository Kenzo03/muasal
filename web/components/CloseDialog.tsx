"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { StatusDot } from "./Chips";
import Icon from "./Icon";
import NodePicker from "./NodePicker";
import { api } from "@/lib/api";
import { nodePaths } from "@/lib/nodes";
import type { components } from "@/lib/api-types";
import { useProblemText, type Node, type Problem, type Status, type Ticket } from "@/lib/problem";
import { button, cx, field } from "@/lib/ui";
import { isWeak } from "@/lib/weak";

type Props = {
  ticket: Ticket; // as read just now: its version guards the close
  status: Status; // the Done or Cancelled status it moves to
  nodes: Node[]; // the project's menus, for a ticket without any
  onDone: () => void; // closed; the caller refreshes
  onCancel: () => void; // nothing changed
  draft?: components["schemas"]["DecisionDraft"]; // an AI draft made beforehand, as the close-out queue makes them (MSL-65)
};

// The words for a length error, by the field the server names.
const lengths: Record<string, string> = {
  reason: "reasonLength",
  "decision.what_changed": "whatChangedLength",
  "decision.why": "whyLength",
  "decision.alternatives": "alternativesLength",
};

// The close dialog (FSD §9.1): moving a ticket into Done or Cancelled asks what
// changed, why and what was rejected, prefilled so it takes seconds (Goal 2),
// plus a reason and menus when the ticket has none. Nothing changes until
// "Close ticket"; Cancel, Escape and × leave the ticket as it was.
export default function CloseDialog({ ticket, status, nodes, onDone, onCancel, draft }: Props) {
  const t = useTranslations("close");
  const tt = useTranslations("ticket");
  const problemText = useProblemText();
  const ref = useRef<HTMLDialogElement>(null);
  const done = status.category === "done";
  const prior = ticket.decision; // a reopened ticket's draft, or the record of an earlier close (R-DC-6)
  const needsReason = ticket.reason.trim() === "";
  const needsMenus = ticket.nodes.length === 0;
  const pathOf = useMemo(() => nodePaths(nodes), [nodes]);
  const [reason, setReason] = useState("");
  const [menus, setMenus] = useState<Set<number>>(() => new Set());
  const [whatChanged, setWhatChanged] = useState(prior?.what_changed ?? (done ? ticket.title : t("notImplemented")));
  const [why, setWhy] = useState(prior?.why ?? ticket.reason);
  const [alternatives, setAlternatives] = useState(prior?.alternatives ?? "");
  const [problem, setProblem] = useState<Problem>();
  const [busy, setBusy] = useState(false);
  // "Draft with AI" (§9.3) replaces only prefills the user has not typed over.
  const locale = useLocale();
  const [typed, setTyped] = useState<Set<string>>(() => new Set());
  const [drafting, setDrafting] = useState(false);
  const [drafted, setDrafted] = useState<{ model: string; noWhy: boolean }>();
  const [draftWhy, setDraftWhy] = useState(""); // the draft's why, offered when the field already has one (MSL-8)
  const typing = (name: string, set: (v: string) => void) => (v: string) => {
    setTyped((s) => new Set(s).add(name));
    set(v);
  };
  useEffect(() => {
    if (!ref.current?.open) ref.current?.showModal();
    if (draft) applyDraft(draft);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- a draft from before the dialog fills it once
  }, []);

  // AC-DC-2: "Close ticket" waits until every required field has text.
  const ready = (!needsReason || reason.trim() !== "") && (!needsMenus || menus.size > 0) && whatChanged.trim() !== "" && why.trim() !== "";
  const serverError = (name: string) => {
    const e = problem?.errors?.find((f) => f.field === name);
    if (!e) return undefined;
    if (name === "node_ids") return t("menusRequired");
    return e.code === "required" ? t("required") : t(lengths[name]);
  };
  const toggleMenu = (id: number, on: boolean) =>
    setMenus((s) => {
      const next = new Set(s);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });

  async function draftWithAI() {
    setDrafting(true);
    const { data, error } = await api.POST("/tickets/{key}/decision-draft", {
      params: { path: { key: ticket.key } },
      body: { language: locale === "en" ? "en" : "id" },
    });
    setDrafting(false);
    if (error) return setProblem(error);
    setProblem(undefined);
    applyDraft(data);
  }

  function applyDraft(data: components["schemas"]["DecisionDraft"]) {
    const fill = (name: string, value: string, current: string, set: (v: string) => void) => {
      if (!typed.has(name) || current.trim() === "") set(value);
    };
    fill("what", data.what_changed, whatChanged, setWhatChanged);
    // AC-DC-7: no reason in the thread empties an untyped prefill, so the
    // requester gets asked. MSL-8: otherwise a why already there keeps its
    // facts, and the draft's differing why is offered beside it, not over it.
    if (data.why.trim() === "" || why.trim() === "") fill("why", data.why, why, setWhy);
    else setDraftWhy(data.why.trim() !== why.trim() ? data.why.trim() : "");
    fill("alternatives", data.alternatives, alternatives, setAlternatives);
    setDrafted({ model: data.model, noWhy: data.why.trim() === "" });
  }

  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    const { error } = await api.POST("/tickets/{key}/transition", {
      params: { path: { key: ticket.key }, header: { "If-Match": `"${ticket.version}"` } },
      body: {
        status_id: status.id,
        reason: needsReason ? reason : undefined,
        node_ids: needsMenus ? [...menus] : undefined,
        decision: { what_changed: whatChanged, why, alternatives, ai_drafted: drafted !== undefined },
      },
    });
    setBusy(false);
    if (error) return setProblem(error);
    onDone();
  }

  const context = [ticket.title, ticket.client?.name ?? tt("core"), ...ticket.nodes.map((n) => pathOf(n.id) || n.name)];
  return (
    <dialog
      ref={ref}
      aria-labelledby="close-title"
      onCancel={(e) => {
        e.preventDefault();
        onCancel();
      }}
      className="m-auto w-[min(620px,calc(100vw-2rem))] rounded-2xl bg-white p-0 text-ink shadow-[0_24px_64px_rgba(43,36,32,0.22)] backdrop:bg-ink/35 backdrop:backdrop-blur-[2px]"
    >
      <form onSubmit={submit} aria-label={t("form")} className="flex max-h-[calc(100vh-4rem)] flex-col">
        <div className="flex flex-col gap-1.5 border-b border-line-soft px-6 pb-4 pt-5">
          <div className="flex items-center gap-2.5">
            <StatusDot color={status.color} className="size-2.5" />
            <h2 id="close-title" className="text-[17px] font-extrabold tracking-[-0.01em]">{t("title", { key: ticket.key, status: status.name })}</h2>
            <button
              type="button"
              onClick={onCancel}
              aria-label={t("dismiss")}
              className="-mr-1.5 ml-auto inline-flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-lg text-muted hover:bg-well hover:text-ink"
            >
              <Icon name="x" />
            </button>
          </div>
          <p className="text-[13px] text-muted">{context.join(" · ")}</p>
        </div>
        <div className="flex flex-col gap-4 overflow-y-auto px-6 py-5">
          {needsReason && (
            <Area id="close-reason" label={t("reason")} value={reason} onChange={setReason} max={2000} rows={2} weak error={serverError("reason")} />
          )}
          {needsMenus && (
            <div className="flex flex-col gap-2">
              <span className="text-[13px] font-bold">{t("menus")}</span>
              <NodePicker nodes={nodes} selected={menus} onToggle={toggleMenu} legend={t("menus")} />
              <p className={cx("text-xs", menus.size === 0 || serverError("node_ids") ? "text-danger" : "text-muted")}>
                {serverError("node_ids") ?? (menus.size === 0 ? t("menusRequired") : t("menusChosen", { count: menus.size }))}
              </p>
            </div>
          )}
          <Area
            id="close-what"
            label={done ? t("whatChanged") : t("whatDecided")}
            value={whatChanged}
            onChange={typing("what", setWhatChanged)}
            max={1000}
            rows={2}
            hint={prior ? t("fromRecord") : done ? t("fromTitle") : undefined}
            error={serverError("decision.what_changed")}
          />
          <Area
            id="close-why"
            label={t("why")}
            value={why}
            onChange={typing("why", setWhy)}
            max={2000}
            rows={3}
            weak
            highlight={drafted?.noWhy === true && why.trim() === ""}
            hint={drafted?.noWhy && why.trim() === "" ? t("noWhy") : prior ? t("fromRecord") : ticket.reason ? t("fromReason") : undefined}
            error={serverError("decision.why")}
          />
          {draftWhy && (
            <div className="-mt-1 flex flex-wrap items-baseline gap-x-2 gap-y-1 rounded-lg bg-well px-3 py-2 text-xs text-ink-soft">
              <span className="font-semibold">{t("draftWhy")}</span>
              <span className="min-w-0 flex-1">{draftWhy}</span>
              <button type="button" className="font-semibold text-accent-strong" onClick={() => { setWhy(draftWhy); setDraftWhy(""); }}>{t("useDraftWhy")}</button>
            </div>
          )}
          <Area
            id="close-alternatives"
            label={t("alternatives")}
            value={alternatives}
            onChange={typing("alternatives", setAlternatives)}
            max={2000}
            rows={2}
            optional
            error={serverError("decision.alternatives")}
          />
          {drafted && <p role="status" className="text-xs text-muted">{t("drafted", { model: drafted.model })}</p>}
          {problem && !problem.errors?.length && <p role="alert" className={field.error}>{problemText(problem)}</p>}
        </div>
        <div className="flex flex-wrap items-center gap-2 rounded-b-2xl border-t border-line-soft bg-paper px-6 py-3.5">
          <span className="text-xs font-semibold text-muted">{t("outcome")}</span>
          <span className={cx("inline-flex items-center gap-1 rounded-full pl-1.5 pr-2.5 text-[11.5px] font-semibold leading-[22px]", done ? "bg-ok-soft text-ok" : "bg-well text-ink-soft")}>
            <Icon name={done ? "check" : "xCircle"} className="size-3.5" />
            {done ? t("implemented") : t("rejected")}
          </span>
          <button type="button" onClick={draftWithAI} disabled={drafting} className={cx(button.secondary, "ml-auto")}>
            <Icon name="sparkle" className="size-4 text-accent" />
            {drafting ? t("drafting") : t("draftWithAI")}
          </button>
          <button type="button" onClick={onCancel} className={button.secondary}>{t("cancel")}</button>
          <button disabled={!ready || busy} className={button.primary}>{t("submit")}</button>
        </div>
      </form>
    </dialog>
  );
}

// One text field of the dialog: its label and counter, then a line saying what
// is wrong, the weak-reason hint (R-DC-8) or where the prefill came from.
function Area({ id, label, value, onChange, max, rows, hint, weak, optional, error, highlight }: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  max: number;
  rows: number;
  hint?: string;
  weak?: boolean;
  optional?: boolean;
  error?: string;
  highlight?: boolean; // AI found no reason in the thread (AC-DC-7)
}) {
  const t = useTranslations("close");
  const tf = useTranslations("ticketForm");
  const missing = !optional && value.trim() === "";
  const soft = weak === true && isWeak(value);
  const note = error ?? (highlight ? hint : missing ? t("required") : soft ? tf("weakReason") : hint);
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-baseline gap-2">
        <label htmlFor={id} className="text-[13px] font-bold">{label}</label>
        <span className="ml-auto text-[11.5px] font-medium tabular-nums text-muted">{optional ? t("optional") : t("counter", { count: value.length, max })}</span>
      </div>
      <textarea
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={rows}
        maxLength={max}
        aria-describedby={note ? `${id}-note` : undefined}
        className={cx(field.textarea, highlight && "border-warn-line bg-warn-soft")}
      />
      {note && (
        <p id={`${id}-note`} className={cx("text-xs", error || (missing && !highlight) ? "text-danger" : soft || highlight ? "text-warn" : "text-muted")}>{note}</p>
      )}
    </div>
  );
}
