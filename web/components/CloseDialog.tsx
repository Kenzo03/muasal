"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { StatusDot } from "./Chips";
import Icon from "./Icon";
import NodePicker from "./NodePicker";
import { api } from "@/lib/api";
import { nodePaths } from "@/lib/nodes";
import { useProblemText, type Node, type Problem, type Status, type Ticket } from "@/lib/problem";
import { button, chip, cx, field } from "@/lib/ui";
import { isWeak } from "@/lib/weak";

type Props = {
  ticket: Ticket; // as read just now: its version guards the close
  status: Status; // the Done or Cancelled status it moves to
  nodes: Node[]; // the project's menus, for a ticket without any
  onDone: () => void; // closed; the caller refreshes
  onCancel: () => void; // nothing changed
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
export default function CloseDialog({ ticket, status, nodes, onDone, onCancel }: Props) {
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
  useEffect(() => {
    if (!ref.current?.open) ref.current?.showModal();
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

  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    const { error } = await api.POST("/tickets/{key}/transition", {
      params: { path: { key: ticket.key }, header: { "If-Match": `"${ticket.version}"` } },
      body: {
        status_id: status.id,
        reason: needsReason ? reason : undefined,
        node_ids: needsMenus ? [...menus] : undefined,
        decision: { what_changed: whatChanged, why, alternatives },
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
      className="m-auto w-[min(620px,calc(100vw-2rem))] rounded-md bg-white p-0 text-ink shadow-2xl backdrop:bg-ink/55"
    >
      <form onSubmit={submit} aria-label={t("form")} className="flex max-h-[calc(100vh-4rem)] flex-col">
        <div className="flex flex-col gap-1 border-b border-line px-[22px] pb-3.5 pt-[18px]">
          <div className="flex items-center gap-2.5">
            <StatusDot color={status.color} className="size-2.5" />
            <h2 id="close-title" className="text-lg font-semibold">{t("title", { key: ticket.key, status: status.name })}</h2>
            <button
              type="button"
              onClick={onCancel}
              aria-label={t("dismiss")}
              className="ml-auto inline-flex size-8 cursor-pointer items-center justify-center rounded text-muted hover:bg-paper"
            >
              <Icon name="x" />
            </button>
          </div>
          <p className="text-[13px] text-muted">{context.join(" · ")}</p>
        </div>
        <div className="flex flex-col gap-3.5 overflow-y-auto px-[22px] py-4">
          {needsReason && (
            <Area id="close-reason" label={t("reason")} value={reason} onChange={setReason} max={2000} rows={2} weak error={serverError("reason")} />
          )}
          {needsMenus && (
            <div className="flex flex-col gap-1.5">
              <span className="text-[13px] font-semibold">{t("menus")}</span>
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
            onChange={setWhatChanged}
            max={1000}
            rows={2}
            hint={prior ? t("fromRecord") : done ? t("fromTitle") : undefined}
            error={serverError("decision.what_changed")}
          />
          <Area
            id="close-why"
            label={t("why")}
            value={why}
            onChange={setWhy}
            max={2000}
            rows={3}
            weak
            hint={prior ? t("fromRecord") : ticket.reason ? t("fromReason") : undefined}
            error={serverError("decision.why")}
          />
          <Area
            id="close-alternatives"
            label={t("alternatives")}
            value={alternatives}
            onChange={setAlternatives}
            max={2000}
            rows={2}
            optional
            error={serverError("decision.alternatives")}
          />
          {problem && !problem.errors?.length && <p role="alert" className={field.error}>{problemText(problem)}</p>}
        </div>
        <div className="flex items-center gap-2 rounded-b-md border-t border-line bg-paper px-[22px] py-3">
          <span className="text-xs text-muted">{t("outcome")}</span>
          <span className={cx(chip, done ? "bg-ok-soft text-ok" : "bg-well text-[#4A423C]")}>{done ? t("implemented") : t("rejected")}</span>
          <button type="button" onClick={onCancel} className={cx(button.secondary, "ml-auto")}>{t("cancel")}</button>
          <button disabled={!ready || busy} className={button.primary}>{t("submit")}</button>
        </div>
      </form>
    </dialog>
  );
}

// One text field of the dialog: its label and counter, then a line saying what
// is wrong, the weak-reason hint (R-DC-8) or where the prefill came from.
function Area({ id, label, value, onChange, max, rows, hint, weak, optional, error }: {
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
}) {
  const t = useTranslations("close");
  const tf = useTranslations("ticketForm");
  const missing = !optional && value.trim() === "";
  const soft = weak === true && isWeak(value);
  const note = error ?? (missing ? t("required") : soft ? tf("weakReason") : hint);
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline gap-2">
        <label htmlFor={id} className="text-[13px] font-semibold">{label}</label>
        <span className="ml-auto font-mono text-[11px] text-muted">{optional ? t("optional") : t("counter", { count: value.length, max })}</span>
      </div>
      <textarea
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={rows}
        maxLength={max}
        aria-describedby={note ? `${id}-note` : undefined}
        className={field.textarea}
      />
      {note && (
        <p id={`${id}-note`} className={cx("text-xs", error || missing ? "text-danger" : soft ? "text-warn" : "text-muted")}>{note}</p>
      )}
    </div>
  );
}
