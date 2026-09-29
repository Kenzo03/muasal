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

// cells holds the line's parts when wide gaps split it into columns.
type Line = { text: string; size: number; cells?: string[] };

// PDF has no headings, only text in fonts of some size. A line is a heading
// when it is short and either set larger than the body text, or numbered like
// "7.4 Node page"; its number's depth sets its level. A gap wider than two
// characters splits a line into cells, so table rows stay rows.
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
    let cells: string[] = [];
    let size = 0;
    let y: number | undefined;
    let end: number | undefined; // where the last visible item ends
    const flush = () => {
      const parts = cells.map((c) => c.replace(/\s+/g, " ").trim()).filter(Boolean);
      if (parts.length > 0) lines.push({ text: parts.join(" "), size, cells: parts });
      cells = [];
      size = 0;
      end = undefined;
    };
    for (const item of content.items) {
      if (!("str" in item)) continue;
      const [, , , scale, x, iy] = item.transform;
      if (y !== undefined && Math.abs(iy - y) > 2) flush();
      y = iy;
      const fontSize = Math.abs(scale) || item.height;
      const visible = item.str.trim() !== "";
      if (visible && (end === undefined || x - end > fontSize * 2)) cells.push(item.str);
      else if (cells.length > 0) cells[cells.length - 1] += item.str;
      if (visible) end = x + item.width;
      size = Math.max(size, fontSize);
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
    // Three cells, or two that are not a section number and its title ("1.<tab>Scope",
    // "3.2<tab>Appendix"), make a table row, never a heading. A lone "3" is a No column.
    const cells = l.cells ?? [l.text];
    if (cells.length > 2 || (cells.length === 2 && !/^\d+(?:\.\d+)*\.$|^\d+(?:\.\d+)+$/.test(cells[0]))) {
      out.push(`| ${cells.join(" | ")} |`);
      continue;
    }
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
