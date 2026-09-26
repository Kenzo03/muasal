"use client";

import { useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { cx, field } from "@/lib/ui";

type Member = components["schemas"]["Mentionable"];

// A comment box that suggests project members who can see the ticket after
// "@" (FSD §8.7); picking one writes "@handle", which notifies them (§8.10).
export default function MentionBox({ ticketKey, onPaste, ...props }: React.TextareaHTMLAttributes<HTMLTextAreaElement> & { ticketKey: string }) {
  const t = useTranslations("activity");
  const ref = useRef<HTMLTextAreaElement>(null);
  const [members, setMembers] = useState<Member[] | null>(null);
  const [query, setQuery] = useState<string | null>(null);
  const [active, setActive] = useState(0);

  async function load() {
    if (members) return;
    const { data } = await api.GET("/tickets/{key}/mentionable", { params: { path: { key: ticketKey } } });
    setMembers(data?.items ?? []);
  }

  function onInput(e: React.FormEvent<HTMLTextAreaElement>) {
    const el = e.currentTarget;
    const before = el.value.slice(0, el.selectionStart);
    const m = before.match(/(?:^|\s)@([\w.-]*)$/);
    if (m) {
      load();
      setQuery(m[1].toLowerCase());
      setActive(0);
    } else setQuery(null);
  }

  const matches = query === null ? [] : (members ?? []).filter((m) => m.handle.startsWith(query) || m.name.toLowerCase().includes(query)).slice(0, 6);

  function pick(m: Member) {
    const el = ref.current!;
    const start = el.value.slice(0, el.selectionStart).replace(/@([\w.-]*)$/, `@${m.handle} `);
    el.value = start + el.value.slice(el.selectionStart);
    el.selectionStart = el.selectionEnd = start.length;
    el.focus();
    setQuery(null);
  }

  return (
    <div className="relative">
      <textarea
        {...props}
        ref={ref}
        onPaste={onPaste}
        onInput={onInput}
        onKeyDown={(e) => {
          if (matches.length === 0) return;
          if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault();
            setActive((a) => (a + (e.key === "ArrowDown" ? 1 : matches.length - 1)) % matches.length);
          } else if (e.key === "Enter" || e.key === "Tab") {
            e.preventDefault();
            pick(matches[active]);
          } else if (e.key === "Escape") setQuery(null);
        }}
        className={cx(field.textarea, "w-full")}
      />
      {matches.length > 0 && (
        <ul role="listbox" aria-label={t("mentions")} className="absolute left-0 top-full z-20 mt-1 w-64 rounded border border-line bg-white py-1 text-[13px] shadow-lg">
          {matches.map((m, i) => (
            <li key={m.id} role="option" aria-selected={i === active}>
              <button type="button" onMouseDown={(e) => { e.preventDefault(); pick(m); }} className={cx("flex w-full cursor-pointer items-baseline gap-2 px-3 py-1.5 text-left hover:bg-paper", i === active && "bg-paper")}>
                <span className="font-medium">{m.name}</span>
                <span className="text-xs text-muted">@{m.handle}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
