"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Client, type Node } from "@/lib/problem";

type Props = {
  projectKey: string;
  node?: Node; // edit this node; without it the form creates one
  parentId?: number;
  parentPath?: string;
  clients: Client[];
  onSaved: (id: number) => void;
  onCancel?: () => void;
};

// Creates or edits a module or menu. The client scope applies to menus only (R-MR-7).
export default function NodeForm({ projectKey, node, parentId, parentPath, clients, onSaved, onCancel }: Props) {
  const t = useTranslations("modules");
  const problemText = useProblemText();
  const [type, setType] = useState<Node["type"]>(node?.type ?? (parentId === undefined ? "module" : "menu"));
  const [specific, setSpecific] = useState(node?.client_specific ?? false);
  const [error, setError] = useState("");

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const clientSpecific = type === "menu" && specific;
    const body = {
      name: String(form.get("name")),
      type,
      code: String(form.get("code")),
      aliases: String(form.get("aliases")).split(","),
      description: String(form.get("description")),
      client_specific: clientSpecific,
      client_ids: clientSpecific ? form.getAll("client_ids").map(Number) : [],
    };
    if (node) {
      const { data, error } = await api.PATCH("/nodes/{id}", { params: { path: { id: node.id } }, body });
      if (error) return setError(problemText(error));
      onSaved(data.id);
    } else {
      const { data, error } = await api.POST("/projects/{key}/nodes", {
        params: { path: { key: projectKey } },
        body: { ...body, parent_id: parentId },
      });
      if (error) return setError(problemText(error));
      onSaved(data.id);
    }
  }

  const input = "rounded border px-3 py-2";
  const title = node ? t("editTitle", { name: node.name }) : t("newTitle");
  return (
    <form aria-label={title} onSubmit={onSubmit} className="flex flex-col gap-3">
      <h2 className="font-medium">{title}</h2>
      {parentPath && <p className="text-sm text-neutral-600">{t("under", { path: parentPath })}</p>}
      <label className="flex flex-col gap-1 text-sm">
        {t("name")}
        <input name="name" defaultValue={node?.name} required maxLength={200} className={input} />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("type")}
        <select name="type" value={type} onChange={(e) => setType(e.target.value === "menu" ? "menu" : "module")} className={input}>
          <option value="module">{t("module")}</option>
          <option value="menu">{t("menu")}</option>
        </select>
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("code")}
        <input name="code" defaultValue={node?.code ?? ""} maxLength={100} className={`${input} font-mono`} />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("aliases")}
        <input name="aliases" defaultValue={node?.aliases.join(", ")} className={input} />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("description")}
        <textarea name="description" defaultValue={node?.description} maxLength={5000} rows={3} className={input} />
      </label>
      {type === "menu" && (
        <fieldset className="flex flex-col gap-2 text-sm">
          <legend className="mb-1">{t("scope")}</legend>
          <label className="flex items-center gap-2">
            <input type="radio" name="scope" checked={!specific} onChange={() => setSpecific(false)} />
            {t("shared")}
          </label>
          <label className="flex items-center gap-2">
            <input type="radio" name="scope" checked={specific} onChange={() => setSpecific(true)} />
            {t("clientSpecific")}
          </label>
          {specific &&
            (clients.length === 0 ? (
              <p className="text-neutral-600">{t("noLinkedClients")}</p>
            ) : (
              clients.map((c) => (
                <label key={c.id} className="ml-6 flex items-center gap-2">
                  <input type="checkbox" name="client_ids" value={c.id} defaultChecked={node?.clients.some((x) => x.id === c.id)} />
                  {c.name}
                </label>
              ))
            ))}
        </fieldset>
      )}
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <div className="flex gap-3">
        <button className="rounded bg-neutral-900 px-4 py-2 text-white">{t("save")}</button>
        {onCancel && (
          <button type="button" onClick={onCancel} className="rounded border px-4 py-2">
            {t("cancel")}
          </button>
        )}
      </div>
    </form>
  );
}

// What members and viewers see of a node; editing is for project admins.
export function ReadOnlyNode({ node }: { node: Node }) {
  const t = useTranslations("modules");
  return (
    <div className="flex flex-col gap-3">
      <h2 className="font-medium">{node.name}</h2>
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
        <dt className="text-neutral-500">{t("type")}</dt>
        <dd>{t(node.type)}</dd>
        <dt className="text-neutral-500">{t("code")}</dt>
        <dd className="font-mono">{node.code ?? "—"}</dd>
        <dt className="text-neutral-500">{t("aliasesTitle")}</dt>
        <dd>{node.aliases.join(", ") || "—"}</dd>
        <dt className="text-neutral-500">{t("description")}</dt>
        <dd className="whitespace-pre-wrap">{node.description || "—"}</dd>
      </dl>
    </div>
  );
}
