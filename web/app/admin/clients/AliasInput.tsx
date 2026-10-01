"use client";

import { useState } from "react";
import Icon from "@/components/Icon";
import { addAlias, maxAliases } from "@/lib/admin";

type Props = {
  id: string;
  value: string[];
  onChange: (aliases: string[]) => void;
  labels: { placeholder: string; remove: (alias: string) => string };
};

// Aliases as chips: Enter or a comma adds what was typed, × removes one,
// Backspace in the empty box removes the last. What's typed but not yet added
// is added on blur, so a save doesn't lose it.
export default function AliasInput({ id, value, onChange, labels }: Props) {
  const [draft, setDraft] = useState("");
  const commit = () => {
    const next = addAlias(value, draft);
    if (next !== value) onChange(next);
    setDraft("");
  };
  return (
    <div className="flex min-h-[42px] flex-wrap items-center gap-1.5 rounded-[10px] border border-field bg-white px-2 py-1.5 focus-within:outline-2 focus-within:outline-accent">
      {value.map((alias) => (
        <span key={alias} className="inline-flex h-7 items-center gap-0.5 rounded-lg bg-well pl-2.5 pr-1 text-[13px] font-semibold text-ink">
          {alias}
          <button type="button" aria-label={labels.remove(alias)} onClick={() => onChange(value.filter((a) => a !== alias))} className="inline-flex size-[22px] items-center justify-center rounded-md text-muted hover:bg-line hover:text-ink">
            <Icon name="x" className="size-3" />
          </button>
        </span>
      ))}
      <input
        id={id}
        value={draft}
        disabled={value.length >= maxAliases}
        placeholder={labels.placeholder}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === ",") {
            e.preventDefault();
            commit();
          } else if (e.key === "Backspace" && draft === "" && value.length > 0) {
            onChange(value.slice(0, -1));
          }
        }}
        className="h-7 min-w-36 flex-1 bg-transparent text-[13.5px] text-ink outline-none placeholder:text-muted"
      />
    </div>
  );
}
