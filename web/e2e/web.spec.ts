import { expect, test } from "@playwright/test";
import { drag, setPassword, signIn } from "./helpers";

// A 1×1 PNG, pasted as the clipboard's only file.
const PNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==";

// FSD §21 Iteration 4b: the create modal behind `c` (§8.1), recently used menus
// first, Markdown with pasted images, and moving menus by dragging (§7.3).
test("a PM files a ticket from the keyboard, pastes a screenshot and reorganises menus", async ({ page }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36).toUpperCase();
  const key = `W${run.slice(-6)}`;
  const password = "e2e-web-admin-passphrase-7";

  await setPassword(page, process.env.E2E_WEB_ADMIN_LINK!, password);
  await signIn(page, process.env.E2E_WEB_ADMIN_EMAIL!, password);
  const origin = new URL(page.url()).origin;
  const call = async (method: string, path: string, data?: unknown) => {
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.json();
  };
  await call("POST", "/projects", { key, name: `Web ${run}` });
  const hr = await call("POST", `/projects/${key}/nodes`, { type: "module", name: "HR" });
  await call("POST", `/projects/${key}/nodes`, { type: "menu", name: "Attendance", parent_id: hr.id });
  const leave = await call("POST", `/projects/${key}/nodes`, { type: "menu", name: "Leave Request", parent_id: hr.id });
  const payroll = await call("POST", `/projects/${key}/nodes`, { type: "module", name: "Payroll" });
  await call("POST", `/projects/${key}/tickets`, { type: "change_request", title: "Earlier work on leave", node_ids: [leave.id], reason: "Setup." });

  // `c` on the board opens the create form over the board; Escape goes back to it.
  await page.goto(`/p/${key}/board`);
  await expect(page.locator('[aria-keyshortcuts="c"]')).toBeVisible(); // the listener is attached
  await page.keyboard.press("c");
  const modal = page.getByRole("dialog", { name: `Tiket baru di ${key}` });
  await expect(modal).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/p/${key}/tickets/new$`));
  await page.keyboard.press("Escape");
  await expect(modal).toHaveCount(0);
  await expect(page).toHaveURL(new RegExp(`/p/${key}/board$`));

  // MSL-32: on a menu page, `c` starts the ticket with that menu ticked.
  await page.goto(`/p/${key}/modules/${leave.id}`);
  await expect(page.locator('[aria-keyshortcuts="c"]')).toBeVisible();
  await page.keyboard.press("c");
  await expect(page).toHaveURL(new RegExp(`/p/${key}/tickets/new\\?node_id=${leave.id}$`));
  await expect(modal.getByRole("checkbox", { name: /HR › Leave Request/ })).toBeChecked();
  await page.goto(`/p/${key}/board`);
  await expect(page.locator('[aria-keyshortcuts="c"]')).toBeVisible();

  // The menu used last comes first, marked Terakhir.
  await page.keyboard.press("c");
  const form = modal.getByRole("form", { name: "Tiket baru" });
  const menus = form.getByRole("checkbox");
  await expect(menus.first()).toHaveAccessibleName(/HR › Leave Request\s*Terakhir/);
  await expect(form.getByRole("checkbox", { name: /Attendance/ })).not.toHaveAccessibleName(/Terakhir/);

  // Markdown renders; an image from outside the server stays a link.
  await form.getByLabel("Judul").fill("Leave balance shows the wrong year");
  await form.getByLabel("Deskripsi").fill("Saldo cuti **salah** setelah tutup tahun.\n\n- langkah satu\n\n![luar](https://example.com/x.png)");
  await form.getByRole("checkbox", { name: /HR › Leave Request/ }).check();
  await form.getByLabel("Alasan").fill("Employees cannot plan leave with a wrong balance.");
  await form.getByRole("button", { name: "Buat", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/t/${key}-2$`));
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.locator("strong", { hasText: "salah" })).toBeVisible();
  await expect(page.getByRole("listitem").filter({ hasText: "langkah satu" })).toBeVisible();
  await expect(page.getByRole("link", { name: "luar" })).toHaveAttribute("href", "https://example.com/x.png");
  await expect(page.locator('img[src^="https://"]')).toHaveCount(0);

  // A pasted screenshot uploads as an attachment and lands in the comment as Markdown.
  const comment = page.getByLabel("Tulis komentar");
  await comment.focus();
  await comment.evaluate((el, b64) => {
    const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
    const data = new DataTransfer();
    data.items.add(new File([bytes], "image.png", { type: "image/png" }));
    el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
  }, PNG);
  await expect(comment).toHaveValue(/!\[pasted-\d+\.png\]\(\/api\/v1\/attachments\/\d+\)/);
  await page.getByRole("button", { name: "Kirim" }).click();
  const shot = page.locator('img[src^="/api/v1/attachments/"]');
  await expect(shot).toHaveCount(1);
  await expect.poll(() => shot.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(1); // served, and a real image

  // Dragging Leave Request onto Payroll's middle moves it inside Payroll.
  await page.goto(`/p/${key}/modules`);
  const handle = page.getByRole("button", { name: "Seret Leave Request" });
  const payrollRow = page.getByRole("button", { name: "Seret Payroll" }).locator("..");
  await drag(page, handle, payrollRow);
  await expect.poll(async () => (await call("GET", `/projects/${key}/nodes`)).items.find((n: { id: number }) => n.id === leave.id)?.parent_id).toBe(payroll.id);

  // Dropping Attendance on HR's top third puts it before HR, at the top level.
  const tree = page.getByRole("region", { name: "Pohon modul" });
  await drag(page, page.getByRole("button", { name: "Seret Attendance" }), page.getByRole("button", { name: "Seret HR" }).locator(".."), { x: 0.5, y: 0.15 });
  await expect(tree.getByRole("list").first().locator(":scope > li").first()).toContainText("Attendance");
});
