import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// FSD §21 Iteration 1 exit check: an admin builds the HRIS tree and scopes members by client.
test("an admin builds the HRIS tree and a member scoped to one client sees only its menus", async ({ page, browser }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36).toUpperCase(); // keys and names stay unique across runs on one stack
  const key = `H${run.slice(-6)}`;
  const clientA = `Klien A ${run}`;
  const clientB = `Klien B ${run}`;
  const budiEmail = `budi-tree-${run.toLowerCase()}@example.com`;
  const adminPassword = "e2e-tree-admin-passphrase-3";
  const budiPassword = "e2e-budi-tree-passphrase-4";

  await setPassword(page, process.env.E2E_TREE_ADMIN_LINK!, adminPassword);
  await signIn(page, process.env.E2E_TREE_ADMIN_EMAIL!, adminPassword);

  // A user who will be scoped to Client B.
  await page.getByRole("link", { name: "Pengguna" }).click();
  await page.getByRole("button", { name: "Pengguna baru" }).click();
  const panel = page.getByRole("dialog", { name: "Pengguna baru" });
  await panel.getByLabel("Nama", { exact: true }).fill("Budi Tree");
  await panel.getByLabel("Email", { exact: true }).fill(budiEmail);
  await panel.getByRole("button", { name: "Buat pengguna" }).click();
  const budiLink = await page.getByTestId("setup-link").textContent();

  // Two clients, each from the New client panel.
  await page.getByRole("link", { name: "Klien", exact: true }).click();
  for (const name of [clientA, clientB]) {
    await page.getByRole("button", { name: "Klien baru" }).click();
    const panel = page.getByRole("dialog", { name: "Klien baru" });
    await panel.getByLabel("Nama", { exact: true }).fill(name);
    await panel.getByRole("button", { name: "Buat klien" }).click();
    await expect(panel).toBeHidden();
    await expect(page.getByRole("button", { name, exact: true })).toBeVisible();
  }

  // The project; creating it opens its settings.
  await page.getByRole("link", { name: "Muasal" }).click();
  await page.getByRole("link", { name: "Proyek baru" }).click();
  await page.getByLabel("Kunci").fill(key);
  await page.getByLabel("Nama").fill(`HRIS ${run}`);
  await page.getByRole("button", { name: "Buat proyek" }).click();
  await expect(page).toHaveURL(new RegExp(`/p/${key}/settings$`));

  // Link both clients, then scope Budi to Client B.
  const links = page.getByRole("form", { name: "Klien proyek" });
  await links.getByRole("checkbox", { name: clientA }).check();
  await links.getByRole("checkbox", { name: clientB }).check();
  await links.getByRole("button", { name: "Simpan klien" }).click();
  await expect(links.getByRole("status")).toHaveText("Tersimpan");

  await page.getByRole("form", { name: "Tambah anggota" }).getByLabel("Orang").selectOption(budiEmail); // MSL-21: a pick
  await page.getByRole("button", { name: "Tambah anggota" }).click();
  const budiRow = page.getByRole("row", { name: budiEmail });
  await budiRow.getByLabel("Cakupan klien").selectOption("some");
  await budiRow.getByRole("checkbox", { name: clientB }).check();
  await page.getByRole("button", { name: "Simpan anggota" }).click();
  await expect(page.getByRole("region", { name: "Anggota" }).getByRole("status")).toHaveText("Tersimpan");

  // The tree: HR › Attendance › Overtime Approval (Client A only) and Leave Request (shared).
  await page.getByRole("link", { name: "Modul" }).click();
  const addNode = async (name: string, options: { type?: "module"; clients?: string[] } = {}) => {
    const form = page.getByRole("form", { name: "Modul atau menu baru" });
    await form.getByLabel("Nama").fill(name);
    if (options.type) await form.getByLabel("Jenis").selectOption(options.type);
    if (options.clients) {
      await form.getByRole("radio", { name: "Khusus klien" }).check();
      for (const c of options.clients) await form.getByRole("checkbox", { name: c }).check();
    }
    await form.getByRole("button", { name: "Simpan" }).click();
    await expect(page.getByRole("button", { name, exact: true })).toBeVisible();
  };
  await page.getByRole("button", { name: "Tambah modul" }).click();
  await addNode("HR");
  await page.getByRole("button", { name: "Tambah anak" }).click();
  await addNode("Attendance", { type: "module" });
  await page.getByRole("button", { name: "Tambah anak" }).click();
  await addNode("Overtime Approval", { clients: [clientA] });
  await page.getByRole("button", { name: "Attendance", exact: true }).click();
  await page.getByRole("button", { name: "Tambah anak" }).click();
  await addNode("Leave Request");
  await expect(page.getByText(clientA, { exact: true })).toBeVisible(); // the badge on Overtime Approval

  // Budi, scoped to Client B, sees the shared menu but not Client A's.
  const budi = await (await browser.newContext()).newPage();
  await setPassword(budi, budiLink!, budiPassword);
  await signIn(budi, budiEmail, budiPassword);
  await budi.getByRole("link", { name: key }).click();
  await budi.getByRole("link", { name: "Modul", exact: true }).click(); // a project opens on its board
  await expect(budi.getByRole("button", { name: "Leave Request", exact: true })).toBeVisible();
  await expect(budi.getByRole("button", { name: "Attendance", exact: true })).toBeVisible();
  await expect(budi.getByText("Overtime Approval")).toHaveCount(0);
});
