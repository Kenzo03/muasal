// Deterministic on server and browser, so hydration matches (profile timezones arrive in a later iteration).
export function utc(iso: string) {
  return `${iso.slice(0, 16).replace("T", " ")} UTC`;
}

export function fileSize(bytes: number) {
  return bytes < 1024 * 1024 ? `${Math.max(1, Math.round(bytes / 1024))} KB` : `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}
