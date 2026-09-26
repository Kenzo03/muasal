import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import { setPassword, signIn } from "./helpers";

// FSD P1 part two: an API token for scripts (§14.3, AC-IN-4), the module-tree
// CSV import (§7.5, AC-MR-7), merging a duplicate menu (R-MR-6) and the CSV
// export of the ticket list (§8.5).
test("an admin imports the tree, merges a duplicate, exports tickets and scripts with a token", async ({ page }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36).toUpperCase();
  const key = `T${run.slice(-6)}`;
  const password = "e2e-tools-admin-passphrase-5";

  await setPassword(page, process.env.E2E_TOOLS_ADMIN_LINK!, password);
  await signIn(page, process.env.E2E_TOOLS_ADMIN_EMAIL!, password);
  const origin = new URL(page.url()).origin;
  const call = async (method: string, path: string, data?: unknown) => {
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.status() === 204 ? null : res.json();
  };
  await call("POST", "/projects", { key, name: `Tools ${run}` });

  // §7.5: a duplicate sibling stops the import (AC-MR-7); the fixed file imports.
  await page.goto(`/p/${key}/modules`);
  await page.getByRole("link", { name: "Impor CSV" }).click();
  const csv = "path,type,code,client_scope,clients,aliases\nHR,module,HR,shared,,\nHR > Overtime Approval,menu,HR.OT,shared,,Persetujuan Lembur\nHR > OT Approval,menu,,shared,,\n";
  await page.getByLabel("Berkas CSV").setInputFiles({ name: "tree.csv", mimeType: "text/csv", buffer: Buffer.from(csv + "HR > overtime approval,menu,,shared,,\n") });
  await page.getByRole("button", { name: "Pratinjau" }).click();
  const preview = page.getByRole("region", { name: "Pratinjau impor" });
  await expect(preview.getByText("Baris 5: Nama ganda di bawah HR")).toBeVisible();
  await expect(preview.getByRole("button", { name: "Impor" })).toBeDisabled();
  await page.getByLabel("Berkas CSV").setInputFiles({ name: "tree.csv", mimeType: "text/csv", buffer: Buffer.from(csv) });
  await page.getByRole("button", { name: "Pratinjau" }).click();
  await expect(preview.getByText("3 baru, 0 berubah, 0 tetap.")).toBeVisible();
  await preview.getByRole("button", { name: "Impor" }).click();
  await expect(preview.getByText("Terimpor: 3 baru, 0 berubah.")).toBeVisible();

  // R-MR-6: the duplicate folds into Overtime Approval and becomes its alias.
  const nodes = (await call("GET", `/projects/${key}/nodes`)).items as { id: number; name: string; aliases: string[] }[];
  const dup = nodes.find((n) => n.name === "OT Approval")!;
  const ot = nodes.find((n) => n.name === "Overtime Approval")!;
  const ticket = await call("POST", `/projects/${key}/tickets`, { type: "bug", title: `On the duplicate ${run}`, node_ids: [dup.id] });
  await page.goto(`/p/${key}/modules/${dup.id}?tab=details`);
  const merge = page.getByRole("form", { name: "Gabungkan ke item lain" });
  await merge.getByLabel("Gabungkan ke").selectOption({ label: "HR › Overtime Approval" });
  page.once("dialog", (d) => d.accept());
  await merge.getByRole("button", { name: "Gabungkan" }).click();
  await expect(page).toHaveURL(new RegExp(`/modules/${ot.id}`));
  const moved = await call("GET", `/tickets/${ticket.key}`);
  expect(moved.nodes.map((n: { id: number }) => n.id)).toEqual([ot.id]);
  expect((await call("GET", `/projects/${key}/nodes`)).items.find((n: { id: number }) => n.id === ot.id).aliases).toContain("OT Approval");

  // §8.5: the list's filter downloads as CSV.
  await page.goto(`/p/${key}/tickets`);
  const download = page.waitForEvent("download");
  await page.getByRole("link", { name: "Ekspor CSV" }).click();
  const file = await (await download).path();
  expect(readFileSync(file, "utf8")).toContain(`On the duplicate ${run}`);

  // §14.3 and AC-IN-4: a read-write token files a ticket; a read-only one cannot.
  await page.goto("/settings/tokens");
  const form = page.getByRole("form", { name: "Token baru" });
  await form.getByLabel("Nama").fill("Sync script");
  await form.getByLabel("Akses").selectOption({ label: "Baca dan tulis" });
  await form.getByRole("button", { name: "Buat token" }).click();
  const writeToken = (await page.getByLabel("Token", { exact: true }).textContent())!.trim();
  expect(writeToken).toMatch(/^msl_/);
  await expect(page.getByRole("cell", { name: "Sync script" })).toBeVisible();
  await form.getByLabel("Nama").fill("Reporting");
  await form.getByRole("button", { name: "Buat token" }).click();
  await expect(page.getByLabel("Token", { exact: true })).not.toHaveText(writeToken);
  const readToken = (await page.getByLabel("Token", { exact: true }).textContent())!.trim();

  const api = await page.context().request;
  const asScript = (token: string) =>
    api.post(`/api/v1/projects/${key}/tickets`, { headers: { Authorization: `Bearer ${token}`, Cookie: "" }, data: { type: "bug", title: `Filed by a script ${run}`, node_ids: [ot.id] } });
  expect((await asScript(writeToken)).status()).toBe(201);
  expect((await asScript(readToken)).status()).toBe(403);
});
