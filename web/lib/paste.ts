import type { Problem } from "./problem";

// Pasting an image into a description or comment
// uploads it as one of the ticket's attachments and inserts a markdown image
// at the cursor (FSD §8.1, §8.7). Text pastes are left alone.
export function pasteImages(ticketKey: string, onError: (p?: Problem) => void, onUploaded: () => void) {
  return async (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
    const images = Array.from(e.clipboardData.files).filter((f) => f.type.startsWith("image/"));
    if (images.length === 0) return;
    e.preventDefault();
    const el = e.currentTarget;
    for (const [i, image] of images.entries()) {
      const ext = image.type.split("/")[1]?.replace("jpeg", "jpg") || "png";
      const name = image.name && image.name !== "image.png" ? image.name : `pasted-${Date.now()}${i ? `-${i}` : ""}.${ext}`;
      const form = new FormData();
      form.append("file", new File([image], name, { type: image.type }));
      // openapi-fetch sends JSON, so the multipart upload uses fetch directly (same origin, same cookie).
      const res = await fetch(`/api/v1/tickets/${encodeURIComponent(ticketKey)}/attachments`, { method: "POST", body: form });
      if (!res.ok) return onError(await res.json().catch(() => undefined));
      const { id } = (await res.json()) as { id: number };
      const md = `![${name}](/api/v1/attachments/${id})\n`;
      el.setRangeText(md, el.selectionStart, el.selectionEnd, "end");
      el.dispatchEvent(new Event("input", { bubbles: true }));
    }
    onUploaded();
  };
}
