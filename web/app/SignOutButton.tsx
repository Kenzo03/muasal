"use client";

import { useRouter } from "next/navigation";
import { api } from "@/lib/api";

export default function SignOutButton({ label }: { label: string }) {
  const router = useRouter();
  async function signOut() {
    await api.POST("/auth/logout");
    router.push("/login");
    router.refresh();
  }
  return (
    <button type="button" className="underline" onClick={signOut}>
      {label}
    </button>
  );
}
