"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { useProblemText } from "@/lib/problem";
import { button, chip, cx, field, panel } from "@/lib/ui";

type Draft = components["schemas"]["TreeDraft"];
type TNode = components["schemas"]["TreeDraftNode"];

// Reviewing a tree draft (§7.7): every proposed node shows its source
// sections; the admin renames, retypes, unticks or adds nodes. Nodes the tree
// already has stay as they are. Nothing changes until Apply.
export default function Review({ initial, projectKey }: { initial: Draft; projectKey: string }) {
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
  }

  if (draft.status === "running") {
    const pct = draft.total_parts ? Math.round((100 * draft.done_parts) / draft.total_parts) : 0;
    return (
      <div className={cx(panel, "mx-auto flex max-w-xl flex-col gap-2 p-4")} role="status">
        <p className="text-sm">{t("running", { done: draft.done_parts, total: draft.total_parts })}</p>
        <div className="h-2 overflow-hidden rounded bg-well" aria-hidden>
          <div className="h-full bg-accent" style={{ width: `${pct}%` }} />
        </div>
        <p className="text-xs text-muted">{t("runningHint")}</p>
      </div>
    );
  }
  if (draft.status === "failed") {
    return <p role="alert" className={cx(panel, "mx-auto max-w-xl p-4 text-sm text-danger")}>{t("failed", { code: draft.error ?? "" })}</p>;
  }

  const editable = draft.status === "ready";
  const kept = nodes.filter((n) => n.keep && !n.exists).length;
  const row = (n: TNode, depth: number): React.ReactNode => (
    <li key={n.tmp_id} className="flex flex-col gap-1">
      <div className="flex flex-wrap items-center gap-2 text-[13px]" style={{ paddingLeft: depth * 20 }}>
        <input
          type="checkbox"
          aria-label={t("keep", { name: n.name })}
          checked={n.keep || n.exists}
          disabled={!editable || n.exists}
          onChange={(e) => setKeep(n.tmp_id, e.target.checked)}
        />
        {editable && !n.exists ? (
          <input value={n.name} aria-label={t("name")} maxLength={200} onChange={(e) => update(n.tmp_id, { name: e.target.value })} className={cx(field.compact, "w-56")} />
        ) : (
          <span className="font-medium">{n.name}</span>
        )}
        {editable && !n.exists ? (
          <select value={n.type} aria-label={t("type")} onChange={(e) => update(n.tmp_id, { type: e.target.value as TNode["type"] })} className={field.compact}>
            <option value="module">{t("module")}</option>
            <option value="menu">{t("menu")}</option>
          </select>
        ) : (
          <span className="text-xs text-muted">{t(n.type)}</span>
        )}
        {n.exists && <span className={cx(chip, "bg-well text-[#4A423C]")}>{t("exists")}</span>}
        {n.duplicate && byId.get(n.duplicate) && (
          <span className={cx(chip, "bg-warn-soft text-warn")}>{t("duplicate", { name: byId.get(n.duplicate)!.name })}</span>
        )}
        {n.sections.map((s) => (
          <Link key={s} href={`/documents/${draft.document_key}#s-${s}`} target="_blank" className="font-mono text-[11px] text-muted">
            {draft.document_key}/{s}
          </Link>
        ))}
      </div>
      {(n.description || n.aliases.length > 0) && (
        <p className="text-xs text-muted" style={{ paddingLeft: depth * 20 + 24 }}>
          {n.description}
          {n.aliases.length > 0 && ` · ${t("aliases")}: ${n.aliases.join(", ")}`}
        </p>
      )}
      {(children.get(n.tmp_id) ?? []).length > 0 && <ul className="flex flex-col gap-1.5">{children.get(n.tmp_id)!.map((c) => row(c, depth + 1))}</ul>}
    </li>
  );

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-3">
      <p className="text-[13px] text-muted">{draft.used_ai ? t("introAI") : t("introHeadings")}</p>
      {result && (
        <p role="status" className="rounded border border-ok bg-ok-soft p-3 text-[13px]">
          {t("applied", { created: result.created, linked: result.linked })}{" "}
          {projectKey && <Link href={`/p/${projectKey}/modules`}>{t("openTree")}</Link>}
        </p>
      )}
      {draft.status === "discarded" && <p role="status" className="text-sm text-muted">{t("discarded")}</p>}
      {draft.status === "applied" && !result && <p role="status" className="text-sm text-muted">{t("alreadyApplied")}</p>}
      {nodes.length === 0 ? (
        <p className="text-muted">{t("empty")}</p>
      ) : (
        <ul aria-label={t("proposal")} className={cx(panel, "flex flex-col gap-1.5 p-4")}>{(children.get("") ?? []).map((n) => row(n, 0))}</ul>
      )}
      {editable && (
        <div className="flex flex-wrap items-end gap-2">
          <label className={field.label}>
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
            {t("add")}
          </button>
          <span className="ml-auto text-xs text-muted">{t("willCreate", { count: kept })}</span>
          <button type="button" onClick={discard} className={button.secondary}>{t("discard")}</button>
          <button type="button" onClick={save} disabled={!dirty} className={button.secondary}>{t("save")}</button>
          <button type="button" onClick={apply} disabled={busy} className={button.primary}>{t("apply")}</button>
        </div>
      )}
      {error && <p role="alert" className={field.error}>{error}</p>}
      <p className="text-xs text-muted">{td("reviewHint")}</p>
    </div>
  );
}
