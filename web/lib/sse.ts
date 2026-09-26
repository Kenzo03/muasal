// A parser for server-sent events (FSD §11.6), fed the response body as it
// arrives. It keeps a partial event between chunks and skips comment lines,
// which the server sends every 15 seconds to keep proxies open.

export type SSEEvent = { event: string; data: string };

export class SSEParser {
  private buffer = "";

  /** Adds text and returns every event it completes. */
  push(text: string): SSEEvent[] {
    this.buffer += text.replace(/\r\n?/g, "\n");
    const out: SSEEvent[] = [];
    let end: number;
    while ((end = this.buffer.indexOf("\n\n")) !== -1) {
      const block = this.buffer.slice(0, end);
      this.buffer = this.buffer.slice(end + 2);
      let event = "message";
      const data: string[] = [];
      for (const line of block.split("\n")) {
        if (line === "" || line.startsWith(":")) continue;
        const colon = line.indexOf(":");
        const field = colon === -1 ? line : line.slice(0, colon);
        const value = colon === -1 ? "" : line.slice(colon + 1).replace(/^ /, "");
        if (field === "event") event = value;
        else if (field === "data") data.push(value);
      }
      if (data.length > 0) out.push({ event, data: data.join("\n") });
    }
    return out;
  }
}
