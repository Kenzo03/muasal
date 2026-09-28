"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { nodePaths } from "@/lib/nodes";
import { useProblemText, type Client, type Node } from "@/lib/problem";
import { button, cx, field, panel, sectionTitle } from "@/lib/ui";
import NodeForm, { ReadOnlyNode } from "../NodeForm";

// The Details tab (FSD §7.4): project admins edit the node here; everyone else
// reads it. Archived nodes are read-only until restored in the tree (R-MR-4).
// Admins also merge a duplicate into another node (R-MR-6).
export default function NodeDetails({ projectKey, node, clients, canEdit, nodes = [] }: {
  projectKey: string;
  node: Node;
  clients: Client[];
  canEdit: boolean;
  nodes?: Node[];
}) {
  const router = useRouter();
  return (
    <div className="flex max-w-3xl flex-col gap-4">
      <div className={cx(panel, "p-5")}>
        {canEdit && !node.archived ? (
          <NodeForm projectKey={projectKey} node={node} clients={clients} onSaved={() => router.refresh()} />
        ) : (
          <ReadOnlyNode node={node} />
        )}
      </div>
      {canEdit && !node.archived && <Merge projectKey={projectKey} node={node} nodes={nodes} />}
    </div>
  );
}

function Merge({ projectKey, node, nodes }: { projectKey: string; node: Node; nodes: Node[] }) {
  const t = useTranslations("merge");
  const router = useRouter();
  const problemText = useProblemText();
  const [error, setError] = useState("");
  const pathOf = useMemo(() => nodePaths(nodes), [nodes]);
  // Not the node itself, nor anything under it.
  const below = useMemo(() => {
    const out = new Set([node.id]);
    for (let grew = true; grew; ) {
      grew = false;
      for (const n of nodes) if (n.parent_id != null && out.has(n.parent_id) && !out.has(n.id)) (out.add(n.id), (grew = true));
    }
    return out;
  }, [nodes, node.id]);
  const targets = nodes.filter((n) => !below.has(n.id));

  async function merge(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const into = Number(new FormData(e.currentTarget).get("into_id"));
    if (!into || !window.confirm(t("confirm", { name: node.name, into: pathOf(into) }))) return;
    const { error } = await api.POST("/nodes/{id}/merge", { params: { path: { id: node.id } }, body: { into_id: into } });
    if (error) return setError(problemText(error));
    router.push(`/p/${projectKey}/modules/${into}`);
    router.refresh();
  }

  return (
    <form onSubmit={merge} aria-label={t("title")} className={cx(panel, "flex flex-col gap-2 p-5")}>
      <h2 className={sectionTitle}>{t("title")}</h2>
      <p className="text-[13px] text-muted">{t("hint")}</p>
      <div className="flex flex-wrap items-end gap-2">
        <label className={field.label}>
          {t("into")}
          <select name="into_id" required defaultValue="" className={cx(field.compact, "min-w-64")}>
            <option value="" disabled>{t("choose")}</option>
            {targets.map((n) => (
              <option key={n.id} value={n.id}>{pathOf(n.id)}</option>
            ))}
          </select>
        </label>
        <button className={button.danger}>{t("merge")}</button>
      </div>
      {error && <p role="alert" className={field.error}>{error}</p>}
    </form>
  );
}
