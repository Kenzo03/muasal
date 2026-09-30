"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { button, cx } from "@/lib/ui";

// Copy the status update as Markdown for an email or chat, or print it.
export default function StatusTools({ markdown }: { markdown: string }) {
  const t = useTranslations("status");
  const [copied, setCopied] = useState(false);
  return (
    <div className="ml-auto flex items-center gap-2 print:hidden">
      <span role="status" className="text-xs text-muted">{copied ? t("copied") : ""}</span>
      <button
        type="button"
        className={button.secondary}
        onClick={async () => {
          await navigator.clipboard.writeText(markdown);
          setCopied(true);
        }}
      >
        {t("copy")}
      </button>
      <button type="button" onClick={() => window.print()} className={cx(button.secondary)}>{t("print")}</button>
    </div>
  );
}
