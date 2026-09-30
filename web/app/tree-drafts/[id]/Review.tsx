"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { useProblemText } from "@/lib/problem";
import { button, chip, cx, field, panel } from "@/lib/ui";

type Draft = components["schemas"]["TreeDraft"];
type TNode = components["schemas"]["TreeDraftNode"];

// Reviewing a tree draft (§7.7): every proposed node shows its source
// sections; the admin renames, retypes, unticks or adds nodes. Nodes the tree
// already has stay as they are. Nothing changes until Apply.
export default function Review({ initial, projectKey, sections }: { initial: Draft; projectKey: string; sections: Record<string, string> }) {
  const t = useTranslations("treeDraft");
  const td = useTranslations("documents");
  const router = useRouter();
  const problemText = useProblemText();
  const [draft, setDraft] = useState(initial);
  const [nodes, setNodes] = useState<TNode[]>(initial.proposal);
  const [dirty, setDirty] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<{ created: number; linked: number }>();
  const [busy, setBusy] = useState(false);
  const [newParent, setNewParent] = useState("");

  // R-MR-12: a running draft shows its progress until it is ready.
  useEffect(() => {
    if (draft.status !== "running") return;
    const timer = setInterval(async () => {
      const { data } = await api.GET("/tree-drafts/{id}", { params: { path: { id: draft.id } } });
      if (!data) return;
      setDraft(data);
      if (data.status !== "running") setNodes(data.proposal);
    }, 2000);
    return () => clearInterval(timer);
  }, [draft.status, draft.id]);

  // MSL-51: a running draft shows it's alive: elapsed time and a moving bar.
  // The clock starts after mount, so the server's render matches.
  const [now, setNow] = useState(0);
  useEffect(() => {
    if (draft.status !== "running") return;
    setNow(Date.now());
    const tick = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(tick);
  }, [draft.status]);

  const children = useMemo(() => {
    const m = new Map<string, TNode[]>();
    for (const n of nodes) m.set(n.parent, [...(m.get(n.parent) ?? []), n]);
    return m;
  }, [nodes]);
  const byId = useMemo(() => new Map(nodes.map((n) => [n.tmp_id, n])), [nodes]);
  const update = (id: string, patch: Partial<TNode>) => {
    setDirty(true);
    setNodes((ns) => ns.map((n) => (n.tmp_id === id ? { ...n, ...patch } : n)));
  };
  // Unticking a node unticks what is under it; ticking one ticks its parents.
  const setKeep = (id: string, keep: boolean) => {
    setDirty(true);
    setNodes((ns) => {
      const next = new Map(ns.map((n) => [n.tmp_id, { ...n }]));
      if (keep) {
        for (let n = next.get(id); n; n = next.get(n.parent)) if (!n.exists) n.keep = true;
      } else {
        const down = (x: string) => {
          const n = next.get(x);
          if (n && !n.exists) n.keep = false;
          for (const c of ns.filter((c) => c.parent === x)) down(c.tmp_id);
        };
        down(id);
      }
      return ns.map((n) => next.get(n.tmp_id)!);
    });
  };

  async function save() {
    const { data, error } = await api.PUT("/tree-drafts/{id}", { params: { path: { id: draft.id } }, body: { proposal: nodes } });
    if (error) {
      setError(error.errors?.[0]?.message ?? problemText(error));
      return false;
    }
    setDraft(data);
    setDirty(false);
    setError("");
    return true;
  }
  async function apply() {
    setBusy(true);
    if (dirty && !(await save())) return setBusy(false);
    const { data, error } = await api.POST("/tree-drafts/{id}/apply", { params: { path: { id: draft.id } } });
    setBusy(false);
    if (error) return setError(error.errors?.[0]?.message ?? problemText(error));
    setResult(data);
    setDraft((d) => ({ ...d, status: "applied" }));
    router.refresh();
  }
  async function discard() {
    if (!window.confirm(t("discardConfirm"))) return;
    const { error } = await api.POST("/tree-drafts/{id}/discard", { params: { path: { id: draft.id } } });
    if (error) return setError(problemText(error));
    setDraft((d) => ({ ...d, status: "discarded" }));
    router.refresh();
  }

  if (draft.status === "running") {
    const pct = draft.total_parts ? Math.round((100 * draft.done_parts) / draft.total_parts) : 0;
    return (
      <div className={cx(panel, "flex max-w-xl flex-col gap-3 p-6")} role="status">
        <div className="flex items-center gap-3">
          <span className="flex size-10 shrink-0 animate-pulse items-center justify-center rounded-full bg-accent-soft text-accent">
            <Icon name="sparkle" className="size-5" />
          </span>
          <p className="text-sm font-semibold">
            {t("running", { done: draft.done_parts, total: draft.total_parts })}
            {now > 0 && (
              <span className="ml-1.5 font-normal text-muted">
                {t("elapsed", { seconds: Math.max(0, Math.round((now - Date.parse(draft.created_at)) / 1000)) })}
              </span>
            )}
          </p>
        </div>
        <div className="h-2 overflow-hidden rounded-full bg-well" aria-hidden>
          <div className="h-full animate-pulse rounded-full bg-accent transition-[width] duration-500" style={{ width: `${Math.max(pct, 8)}%` }} />
        </div>
        <p className="text-xs text-muted">{t("runningHint")}</p>
      </div>
    );
  }
  if (draft.status === "failed") {
    return (
      <p role="alert" className="flex max-w-xl items-start gap-2 rounded-xl border border-danger-line bg-danger-soft px-4 py-3 text-sm font-medium text-danger">
        <Icon name="warning" className="mt-0.5 size-4 shrink-0" />
        {t("failed", { code: draft.error ?? "" })}
      </p>
    );
  }

  const editable = draft.status === "ready";
  const kept = nodes.filter((n) => n.keep && !n.exists).length;
  // Each level indents 1.5rem; notes under a row line up with its name.
  const indent = (depth: number) => `${0.5 + depth * 1.5}rem`;
  const row = (n: TNode, depth: number): React.ReactNode => {
    const dropped = !n.keep && !n.exists;
    const twin = n.duplicate ? byId.get(n.duplicate) : undefined;
    const kids = children.get(n.tmp_id) ?? [];
    return (
      <li key={n.tmp_id}>
        <div
          className={cx("flex flex-wrap items-center gap-x-2 gap-y-1 rounded-xl py-1.5 pr-2 hover:bg-paper", dropped && "opacity-50")}
          style={{ paddingLeft: indent(depth) }}
        >
          <input
            type="checkbox"
            aria-label={t("keep", { name: n.name })}
            checked={n.keep || n.exists}
            disabled={!editable || n.exists}
            onChange={(e) => setKeep(n.tmp_id, e.target.checked)}
            className="size-4 shrink-0 accent-accent"
          />
          <Icon name={n.type === "module" ? "folder" : "screen"} className="size-4 shrink-0 text-muted" />
          {editable && !n.exists ? (
            // Reads as text until hovered or focused, so the draft looks like a tree, not a form.
            <input
              value={n.name}
              aria-label={t("name")}
              maxLength={200}
              onChange={(e) => update(n.tmp_id, { name: e.target.value })}
              className={cx(
                "h-8 min-w-0 flex-1 rounded-lg border border-transparent bg-transparent px-2 text-[13.5px] text-ink hover:border-line hover:bg-white focus:border-field focus:bg-white sm:w-60 sm:flex-none",
                n.type === "module" && "font-semibold",
              )}
            />
          ) : (
            <span className={cx("px-2 text-[13.5px]", n.type === "module" && "font-semibold")}>{n.name}</span>
          )}
          {editable && !n.exists ? (
            <select
              value={n.type}
              aria-label={t("type")}
              onChange={(e) => update(n.tmp_id, { type: e.target.value as TNode["type"] })}
              className="h-7 cursor-pointer rounded-lg bg-well pl-2 text-xs font-semibold text-ink-soft hover:text-ink"
            >
              <option value="module">{t("module")}</option>
              <option value="menu">{t("menu")}</option>
            </select>
          ) : (
            <span className="text-xs text-muted">{t(n.type)}</span>
          )}
          {n.code && <span className="font-mono text-xs text-muted">{n.code}</span>}
          {n.exists && (
            <span className={cx(chip, "bg-ok-soft text-ok")}>
              <Icon name="check" className="size-3.5" />
              {t("exists")}
            </span>
          )}
          {twin && (
            <span className={cx(chip, "bg-warn-soft text-warn")}>
              <Icon name="warning" className="size-3.5" />
              {t("duplicate", { name: twin.name })}
            </span>
          )}
          {n.sections.length > 0 && (
            <span className="ml-auto flex flex-wrap gap-1">
              {n.sections.map((s) => (
                <Link
                  key={s}
                  href={`/documents/${draft.document_key}#s-${s}`}
                  target="_blank"
                  title={`${draft.document_key}/${s} · ${sections[s] ?? ""}`}
                  className="inline-flex max-w-64 items-center gap-1 rounded-md bg-well px-1.5 text-[11px] font-semibold leading-5 text-muted no-underline hover:bg-accent-soft hover:text-accent-strong"
                >
                  <Icon name="file" className="size-3 shrink-0" />
                  {/* MSL-3: the heading, not a bare "s1", so a wrong link shows. */}
                  <span className="truncate">{s.startsWith("s") ? (sections[s] ?? s) : `${s} ${sections[s] ?? ""}`.trim()}</span>
                </Link>
              ))}
            </span>
          )}
        </div>
        {(n.description || n.aliases.length > 0) && (
          <p className="pb-1.5 pr-2 text-xs text-muted" style={{ paddingLeft: `calc(${indent(depth)} + 3.5rem)` }}>
            {n.description}
            {n.description && n.aliases.length > 0 && " · "}
            {n.aliases.length > 0 && `${t("aliases")}: ${n.aliases.join(", ")}`}
          </p>
        )}
        {kids.length > 0 && <ul>{kids.map((c) => row(c, depth + 1))}</ul>}
      </li>
    );
  };

  const note = "flex gap-2.5 rounded-xl px-4 py-3 text-[13px]";
  return (
    <div className="flex max-w-4xl flex-col gap-4">
      <p className={cx(note, "items-start bg-well text-ink-soft")}>
        <Icon name={draft.used_ai ? "sparkle" : "list"} className="mt-0.5 size-4 shrink-0 text-accent" />
        {draft.used_ai ? t("introAI") : t("introHeadings")}
      </p>
      {result && (
        <div role="status" className={cx(note, "flex-wrap items-center border border-ok/20 bg-ok-soft font-semibold text-ok")}>
          <Icon name="check" className="size-5 shrink-0" />
          {t("applied", { created: result.created, linked: result.linked })}
          {projectKey && (
            <Link href={`/p/${projectKey}/modules`} className={cx(button.secondary, "ml-auto")}>
              {t("openTree")}
              <Icon name="arrowRight" className="size-3.5" />
            </Link>
          )}
        </div>
      )}
      {draft.status === "discarded" && <p role="status" className={cx(note, "items-center bg-well text-ink-soft")}>{t("discarded")}</p>}
      {draft.status === "applied" && !result && <p role="status" className={cx(note, "items-center bg-well text-ink-soft")}>{t("alreadyApplied")}</p>}
      <div className={cx(panel, "flex flex-col")}>
        {nodes.length === 0 ? (
          <p className="px-6 py-10 text-center text-[13.5px] text-muted">{t("empty")}</p>
        ) : (
          <ul aria-label={t("proposal")} className="p-2">{(children.get("") ?? []).map((n) => row(n, 0))}</ul>
        )}
        {editable && (
          <div className="flex flex-wrap items-center gap-2 border-t border-line-soft px-4 py-3">
            <label className="flex items-center gap-2 text-[13px] font-semibold">
              {t("addUnder")}
              <select value={newParent} onChange={(e) => setNewParent(e.target.value)} className={field.compact}>
                <option value="">{t("top")}</option>
                {nodes.map((n) => (
                  <option key={n.tmp_id} value={n.tmp_id}>{n.name}</option>
                ))}
              </select>
            </label>
            <button
              type="button"
              className={button.secondary}
              onClick={() => {
                setDirty(true);
                const id = `new${Date.now()}`;
                setNodes((ns) => [...ns, { tmp_id: id, parent: newParent, type: newParent ? "menu" : "module", name: t("newName"), aliases: [], description: "", sections: [], keep: true, exists: false }]);
                if (newParent) setKeep(newParent, true);
              }}
            >
              <Icon name="plus" />
              {t("add")}
            </button>
          </div>
        )}
      </div>
      {error && <p role="alert" className={field.error}>{error}</p>}
      <p className="text-xs leading-relaxed text-muted">{td("reviewHint")}</p>
      {editable && (
        // Floats at the bottom of the window while a long draft scrolls.
        <div className="sticky bottom-4 z-10 flex flex-wrap items-center gap-2 rounded-2xl border border-line bg-white/95 px-4 py-3 shadow-[0_8px_32px_rgba(43,36,32,0.14)] backdrop-blur">
          <span className="w-full text-[13.5px] font-bold sm:w-auto">{t("willCreate", { count: kept })}</span>
          <button type="button" onClick={discard} className={cx(button.secondary, "ml-auto")}>{t("discard")}</button>
          <button type="button" onClick={save} disabled={!dirty} className={button.secondary}>{t("save")}</button>
          <button type="button" onClick={apply} disabled={busy} className={button.primary}>{t("apply")}</button>
        </div>
      )}
    </div>
  );
}
