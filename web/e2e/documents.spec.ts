import { expect, test, type Page } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

function api(page: Page) {
  return async (method: string, path: string, data?: unknown) => {
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: new URL(page.url()).origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.status() === 204 ? null : res.json();
  };
}

// pdf builds a one-page PDF whose lines are set in Helvetica at the given sizes.
function pdf(lines: { text: string; size: number }[]): Buffer {
  let y = 800;
  const ops = lines
    .map((l) => {
      y -= l.size + 10;
      return `BT /F1 ${l.size} Tf 50 ${y} Td (${l.text.replace(/[()\\]/g, "\\$&")}) Tj ET`;
    })
    .join("\n");
  const objects = [
    "<< /Type /Catalog /Pages 2 0 R >>",
    "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
    "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
    "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
    `<< /Length ${Buffer.byteLength(ops)} >>\nstream\n${ops}\nendstream`,
  ];
  let out = "%PDF-1.4\n";
  const offsets: number[] = [];
  objects.forEach((o, i) => {
    offsets.push(Buffer.byteLength(out));
    out += `${i + 1} 0 obj\n${o}\nendobj\n`;
  });
  const xref = Buffer.byteLength(out);
  out += `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n${offsets.map((o) => `${String(o).padStart(10, "0")} 00000 n \n`).join("")}`;
  out += `trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return Buffer.from(out, "latin1");
}

// FSD §7.7 with AI off (AC-MR-10): an admin uploads a PDF, which the browser
// converts; its headings become sections; the heading draft proposes the tree,
// and applying it creates only the ticked nodes.
test("an admin drafts the module tree from a PDF", async ({ page }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36).toUpperCase();
  const key = `F${run.slice(-6)}`;
  const password = "e2e-docs-admin-passphrase-5";
  await setPassword(page, process.env.E2E_DOCS_ADMIN_LINK!, password);
  await signIn(page, process.env.E2E_DOCS_ADMIN_EMAIL!, password);
  const call = api(page);
  await call("POST", "/projects", { key, name: `Payroll ${run}` });

  await page.goto(`/p/${key}/documents`);
  await page.getByLabel(/^Berkas/).setInputFiles({
    name: "payroll-fsd.pdf",
    mimeType: "application/pdf",
    buffer: pdf([
      { text: "Payroll FSD", size: 20 },
      { text: "1 Payroll", size: 11 },
      { text: "The payroll module pays every employee each month.", size: 11 },
      { text: "1.1 Payslip", size: 11 },
      { text: "Each employee downloads a monthly payslip.", size: 11 },
      { text: "1.2 Overtime Pay", size: 11 },
      { text: "Overtime is paid at one and a half times the hourly rate.", size: 11 },
    ]),
  });
  await expect(page.getByLabel("Judul", { exact: true })).toHaveValue("payroll-fsd");
  await page.getByRole("button", { name: "Unggah" }).click();

  await page.waitForURL(/\/documents\/F[A-Z0-9]+-DOC1$/);
  await expect(page.getByRole("heading", { name: "1.1 Payslip" })).toBeVisible();
  await expect(page.getByRole("region", { name: "1.2 Overtime Pay" })).toContainText("one and a half times");

  await page.getByRole("button", { name: "Susun draf pohon modul" }).click();
  await page.waitForURL(/\/tree-drafts\/\d+$/);
  const tree = page.getByRole("list", { name: "Usulan pohon" });
  await expect(tree.getByRole("textbox")).toHaveCount(3);
  await page.getByRole("checkbox", { name: "Pertahankan Overtime Pay" }).uncheck();
  await expect(page.getByText("2 node akan dibuat")).toBeVisible();
  await page.getByRole("button", { name: "Terapkan ke pohon" }).click();
  await expect(page.getByRole("status").filter({ hasText: "Diterapkan: 2 node dibuat" })).toBeVisible();

  const nodes = await call("GET", `/projects/${key}/nodes`);
  expect(nodes.items.map((n: { name: string }) => n.name).sort()).toEqual(["Payroll", "Payslip"]);
});
