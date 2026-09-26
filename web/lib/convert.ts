// Documents are converted to Markdown in the browser (FSD §7.7), so the
// server never parses PDF or Word files: pdf.js reads PDF text, mammoth reads
// DOCX, and headings become the sections Muasal stores.

export const documentExtensions = [".pdf", ".docx", ".md", ".markdown"];

export async function toMarkdown(file: File): Promise<string> {
  const name = file.name.toLowerCase();
  if (name.endsWith(".md") || name.endsWith(".markdown")) return file.text();
  if (name.endsWith(".docx")) {
    const mammoth = await import("mammoth");
    const { value } = await mammoth.convertToHtml({ arrayBuffer: await file.arrayBuffer() });
    return htmlToMarkdown(new DOMParser().parseFromString(value, "text/html").body);
  }
  if (name.endsWith(".pdf")) return pdfToMarkdown(await file.arrayBuffer());
  throw new Error("unsupported");
}

// htmlToMarkdown keeps what sections need from mammoth's HTML: headings,
// paragraphs, list items and table rows, as plain text lines.
export function htmlToMarkdown(root: Element): string {
  const out: string[] = [];
  const text = (el: Element) => (el.textContent ?? "").replace(/\s+/g, " ").trim();
  const walk = (el: Element) => {
    for (const child of Array.from(el.children)) {
      const tag = child.tagName.toLowerCase();
      const h = /^h([1-6])$/.exec(tag);
      if (h) out.push("", `${"#".repeat(Number(h[1]))} ${text(child)}`, "");
      else if (tag === "p") out.push(text(child), "");
      else if (tag === "li") out.push(`- ${text(child)}`);
      else if (tag === "tr") out.push(`| ${Array.from(child.children).map(text).join(" | ")} |`);
      else if (tag === "ul" || tag === "ol" || tag === "table" || tag === "tbody" || tag === "thead") {
        walk(child);
        out.push("");
      } else walk(child);
    }
  };
  walk(root);
  return out.join("\n").replace(/\n{3,}/g, "\n\n").trim() + "\n";
}

type Line = { text: string; size: number };

// PDF has no headings, only text in fonts of some size. A line is a heading
// when it is short and either set larger than the body text, or numbered like
// "7.4 Node page"; its number's depth sets its level.
async function pdfToMarkdown(data: ArrayBuffer): Promise<string> {
  const pdfjs = await import("pdfjs-dist");
  if (!pdfjs.GlobalWorkerOptions.workerPort) {
    pdfjs.GlobalWorkerOptions.workerPort = new Worker(new URL("pdfjs-dist/build/pdf.worker.min.mjs", import.meta.url), { type: "module" });
  }
  const doc = await pdfjs.getDocument({ data }).promise;
  const lines: Line[] = [];
  for (let p = 1; p <= doc.numPages; p++) {
    const page = await doc.getPage(p);
    const content = await page.getTextContent();
    let text = "";
    let size = 0;
    let y: number | undefined;
    const flush = () => {
      if (text.trim()) lines.push({ text: text.replace(/\s+/g, " ").trim(), size });
      text = "";
      size = 0;
    };
    for (const item of content.items) {
      if (!("str" in item)) continue;
      const iy = item.transform[5];
      if (y !== undefined && Math.abs(iy - y) > 2) flush();
      y = iy;
      text += item.str;
      size = Math.max(size, Math.abs(item.transform[3]) || item.height);
      if (item.hasEOL) flush();
    }
    flush();
  }
  return linesToMarkdown(lines);
}

export function linesToMarkdown(lines: Line[]): string {
  const weight = new Map<number, number>();
  for (const l of lines) weight.set(Math.round(l.size), (weight.get(Math.round(l.size)) ?? 0) + l.text.length);
  const body = [...weight].sort((a, b) => b[1] - a[1])[0]?.[0] ?? 0;
  const out: string[] = [];
  for (const l of lines) {
    const short = l.text.length <= 100 && !/[.,;:]$/.test(l.text);
    const numbered = /^(\d+(?:\.\d+)*)\.?\s+\p{Lu}/u.exec(l.text);
    const larger = body > 0 && l.size >= body * 1.15;
    if (short && (numbered || larger)) {
      const depth = numbered ? numbered[1].split(".").length : 0;
      out.push("", `${"#".repeat(Math.min(6, depth + 1))} ${l.text}`, "");
    } else {
      out.push(l.text);
    }
  }
  return out.join("\n").replace(/\n{3,}/g, "\n\n").trim() + "\n";
}
