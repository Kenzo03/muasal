"use client";

import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import Icon from "@/components/Icon";

// SetupClose hides a project's setup steps in this browser: the board page
// skips the card for every project key in the `setup-hidden` cookie.
export default function SetupClose({ projectKey }: { projectKey: string }) {
  const t = useTranslations("project.setup");
  const router = useRouter();
  function close() {
    const now = document.cookie.match(/(?:^|; )setup-hidden=([^;]*)/)?.[1] ?? "";
    const keys = new Set(decodeURIComponent(now).split(",").filter(Boolean)).add(projectKey);
    document.cookie = `setup-hidden=${encodeURIComponent([...keys].join(","))}; path=/; max-age=31536000; samesite=lax`;
    router.refresh();
  }
  return (
    <button type="button" aria-label={t("hide")} title={t("hide")} onClick={close}
      className="inline-flex size-7 shrink-0 items-center justify-center rounded-lg text-ink-soft hover:bg-well hover:text-ink">
      <Icon name="x" className="size-4" />
    </button>
  );
}
