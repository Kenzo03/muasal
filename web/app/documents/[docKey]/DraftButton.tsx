"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import { useProblemText } from "@/lib/problem";
import { button } from "@/lib/ui";

// "Draft module tree" (§7.7): project admins start a tree draft from this document.
export default function DraftButton({ docKey }: { docKey: string }) {
  const t = useTranslations("documents");
  const router = useRouter();
  const problemText = useProblemText();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return (
    <>
      <button
        type="button"
        disabled={busy}
        className={button.primary}
        onClick={async () => {
          setBusy(true);
          const { data, error } = await api.POST("/documents/{key}/tree-drafts", { params: { path: { key: docKey } } });
          setBusy(false);
          if (error) return setError(problemText(error));
          router.push(`/tree-drafts/${data.id}`);
        }}
      >
        <Icon name="tree" />
        {t("draftTree")}
      </button>
      {error && <span role="alert" className="text-xs text-danger">{error}</span>}
    </>
  );
}
