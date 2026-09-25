// R-DC-8: a reason or why is weak when, after trimming, it has fewer than 20
// characters, or fewer than 20 once stock phrases such as "sesuai permintaan"
// are taken out. The hint never blocks a save. Empty text is missing, not weak.
const stock = ["per request", "as requested", "client request", "sesuai permintaan", "permintaan klien", "permintaan client", "request user", "ok", "done"];
const phrase = new RegExp(`(?<![\\p{L}\\p{N}])(?:${stock.join("|")})(?![\\p{L}\\p{N}])`, "giu");

export function isWeak(text: string): boolean {
  const trimmed = text.trim();
  if (trimmed === "") return false;
  return trimmed.replace(phrase, "").replace(/\s+/g, " ").trim().length < 20;
}
