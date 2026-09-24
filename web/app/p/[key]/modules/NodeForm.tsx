"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { ClientChip } from "@/components/Chips";
import { api } from "@/lib/api";
import { useProblemText, type Client, type Node } from "@/lib/problem";
import { button, chip, cx, field } from "@/lib/ui";

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

  const title = node ? t("editTitle", { name: node.name }) : t("newTitle");
  return (
    <form aria-label={title} onSubmit={onSubmit} className="flex flex-col gap-3">
      <div>
        <h2 className="text-base font-semibold">{title}</h2>
        {parentPath && <p className="text-xs text-muted">{t("under", { path: parentPath })}</p>}
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <label className={field.label}>
          {t("name")}
          <input name="name" defaultValue={node?.name} required maxLength={200} className={field.input} />
        </label>
        <label className={field.label}>
          {t("type")}
          <select name="type" value={type} onChange={(e) => setType(e.target.value === "menu" ? "menu" : "module")} className={field.input}>
            <option value="module">{t("module")}</option>
            <option value="menu">{t("menu")}</option>
          </select>
        </label>
        <label className={field.label}>
          {t("code")}
          <input name="code" defaultValue={node?.code ?? ""} maxLength={100} className={cx(field.input, "font-mono")} />
        </label>
        <label className={field.label}>
          {t("aliases")}
          <input name="aliases" defaultValue={node?.aliases.join(", ")} className={field.input} />
        </label>
      </div>
      <label className={field.label}>
        {t("description")}
        <textarea name="description" defaultValue={node?.description} maxLength={5000} rows={3} className={field.textarea} />
      </label>
      {type === "menu" && (
        <fieldset className="flex flex-col gap-2 text-[13px]">
          <legend className="mb-1 font-medium">{t("scope")}</legend>
          <div className="flex flex-wrap gap-x-5 gap-y-2">
            <label className="flex items-center gap-2">
              <input type="radio" name="scope" checked={!specific} onChange={() => setSpecific(false)} className="size-4 accent-accent" />
              {t("shared")}
            </label>
            <label className="flex items-center gap-2">
              <input type="radio" name="scope" checked={specific} onChange={() => setSpecific(true)} className="size-4 accent-accent" />
              {t("clientSpecific")}
            </label>
          </div>
          {specific &&
            (clients.length === 0 ? (
              <p className="text-muted">{t("noLinkedClients")}</p>
            ) : (
              <div className="flex flex-wrap gap-x-4 gap-y-2 rounded border border-line-soft bg-paper p-2.5">
                {clients.map((c) => (
                  <label key={c.id} className="flex items-center gap-2">
                    <input type="checkbox" name="client_ids" value={c.id} defaultChecked={node?.clients.some((x) => x.id === c.id)} className="size-4 accent-accent" />
                    {c.name}
                  </label>
                ))}
              </div>
            ))}
        </fieldset>
      )}
      {error && <p role="alert" className={field.error}>{error}</p>}
      <div className="flex gap-2">
        <button className={button.primary}>{t("save")}</button>
        {onCancel && (
          <button type="button" onClick={onCancel} className={button.secondary}>
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
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="text-lg font-semibold">{node.name}</h2>
        <span className="text-[13px] text-muted">{t(node.type)}</span>
        {node.type === "menu" &&
          (node.client_specific ? (
            node.clients.map((c) => <ClientChip key={c.id} client={c} coreLabel="" />)
          ) : (
            <span className={cx(chip, "bg-ground text-[#4A423C]")}>{t("shared")}</span>
          ))}
      </div>
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-[13px]">
        <dt className="text-muted">{t("code")}</dt>
        <dd className="font-mono">{node.code ?? "—"}</dd>
        <dt className="text-muted">{t("aliasesTitle")}</dt>
        <dd>{node.aliases.join(", ") || "—"}</dd>
        <dt className="text-muted">{t("description")}</dt>
        <dd className="whitespace-pre-wrap">{node.description || "—"}</dd>
      </dl>
    </div>
  );
}
