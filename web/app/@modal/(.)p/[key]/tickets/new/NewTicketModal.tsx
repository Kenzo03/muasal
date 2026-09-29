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
      className="m-auto max-h-[92vh] w-[min(820px,calc(100vw-2rem))] overflow-y-auto rounded-2xl bg-white p-0 text-ink shadow-[0_24px_64px_rgba(43,36,32,0.22)] backdrop:bg-ink/35 backdrop:backdrop-blur-[2px]"
    >
      <div className="sticky top-0 z-20 flex items-center gap-2 border-b border-line-soft bg-white px-5 py-3.5 md:px-6">
        <h2 id="new-ticket-title" className="text-[17px] font-extrabold tracking-[-0.01em]">{title}</h2>
        <button
          type="button"
          onClick={() => router.back()}
          aria-label={closeLabel}
          className="-mr-1.5 ml-auto inline-flex size-8 cursor-pointer items-center justify-center rounded-lg text-muted hover:bg-well hover:text-ink"
        >
          <Icon name="x" />
        </button>
      </div>
      <TicketForm {...form} onCancel={() => router.back()} />
    </dialog>
  );
}
