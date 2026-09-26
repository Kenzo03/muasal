"use client";

import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import Answer, { type Chip, type Turn } from "@/components/ask/Answer";
import AskView from "@/components/ask/AskView";
import { api } from "@/lib/api";
import { button } from "@/lib/ui";

// The open thread: its saved answers, then the Ask box for more questions.
export default function AskPage({ threadId, history, chips, question }: { threadId?: number; history: Turn[]; chips: Chip[]; question?: string }) {
  const t = useTranslations("ask");
  const router = useRouter();
  return (
    <div className="flex flex-col gap-4">
      {threadId && (
        <div className="flex max-w-3xl justify-end">
          <button
            type="button"
            className={button.quiet}
            onClick={async () => {
              const { error } = await api.DELETE("/ask/threads/{id}", { params: { path: { id: threadId } } });
              if (!error) {
                router.push("/ask");
                router.refresh();
              }
            }}
          >
            {t("hideThread")}
          </button>
        </div>
      )}
      {history.length > 0 && (
        <div className="flex max-w-3xl flex-col gap-4">
          {history.map((turn, i) => (
            <Answer key={i} turn={turn} />
          ))}
        </div>
      )}
      <AskView
        chips={chips}
        threadId={threadId}
        question={question}
        onThread={() => {
          // Stay on this view, so the live answer keeps its scope and results;
          // the side list picks up the new thread. A Home question is not re-asked on reload.
          if (question) window.history.replaceState(null, "", "/ask");
          router.refresh();
        }}
      />
    </div>
  );
}
