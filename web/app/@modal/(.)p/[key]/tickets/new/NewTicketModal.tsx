"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import Icon from "@/components/Icon";
import TicketForm from "@/components/TicketForm";
import type { Client, Node, Ref } from "@/lib/problem";

type Props = {
  title: string;
  closeLabel: string;
  projectKey: string;
  clients: Client[];
  nodes: Node[];
  assignees: Ref[];
  statusId?: number;
  nodeId?: number;
};

// A native <dialog> around the create form. Closing it (×, Cancel, Escape or
// a click on the backdrop) goes back, so the page under it stays as it was.
export default function NewTicketModal({ title, closeLabel, ...form }: Props) {
  const router = useRouter();
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    if (!ref.current?.open) ref.current?.showModal();
  }, []);
  return (
    <dialog
      ref={ref}
      aria-labelledby="new-ticket-title"
      onCancel={(e) => {
        e.preventDefault();
        router.back();
      }}
      onClick={(e) => {
        if (e.target === e.currentTarget) router.back(); // the backdrop
      }}
      className="m-auto max-h-[92vh] w-[min(820px,96vw)] overflow-y-auto rounded border border-line bg-white p-0 text-ink shadow-xl backdrop:bg-ink/40"
    >
      <div className="sticky top-0 z-10 flex items-center gap-2 border-b border-line bg-white px-5 py-3">
        <h2 id="new-ticket-title" className="text-base font-semibold">{title}</h2>
        <button type="button" onClick={() => router.back()} aria-label={closeLabel} className="ml-auto rounded p-1 text-muted hover:bg-paper hover:text-ink">
          <Icon name="x" className="size-4" />
        </button>
      </div>
      <TicketForm {...form} onCancel={() => router.back()} />
    </dialog>
  );
}
