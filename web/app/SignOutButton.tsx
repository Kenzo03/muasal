"use client";

import { useRouter } from "next/navigation";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";

export default function SignOutButton({ label }: { label: string }) {
  const router = useRouter();
  async function signOut() {
    await api.POST("/auth/logout");
    router.push("/login");
    router.refresh();
  }
  return (
    <button type="button" onClick={signOut} className="flex w-full cursor-pointer items-center gap-2 px-3 py-2 text-left text-sm text-ink hover:bg-paper">
      <Icon name="signOut" className="size-4 text-muted" />
      {label}
    </button>
  );
}
