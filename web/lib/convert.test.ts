import assert from "node:assert/strict";
import test from "node:test";
import { asTables, firstHeading, linesToMarkdown } from "./convert.ts";

test("numbered and larger short lines become headings", () => {
  const md = linesToMarkdown([
    { text: "Payroll FSD", size: 20 },
    { text: "1 Payroll", size: 11 },
    { text: "The payroll runs monthly and pays every employee on time.", size: 11 },
    { text: "1.2 Payslip", size: 11 },
    { text: "Each employee gets a payslip.", size: 11 },
    { text: "1. Upload the file.", size: 11 },
  ]);
  assert.equal(
    md,
    "# Payroll FSD\n\n## 1 Payroll\n\nThe payroll runs monthly and pays every employee on time.\n\n### 1.2 Payslip\n\nEach employee gets a payslip.\n1. Upload the file.\n",
  );
});

// A numbered first cell ("1 | Hartono | Project Sponsor") once made a table row a heading.
test("table rows stay rows; a number set apart from its title is still a heading", () => {
  const md = linesToMarkdown([
    { text: "1. Document Author", size: 11, cells: ["1.", "Document Author"] },
    { text: "1 Hartono Project Sponsor", size: 11, cells: ["1", "Hartono", "Project Sponsor"] },
    { text: "3 Willy Project Manager", size: 11, cells: ["3", "Willy Project Manager"] },
    { text: "3.2 Appendix", size: 11, cells: ["3.2", "Appendix"] },
    { text: "E-MSTK-01 Dashboard", size: 11, cells: ["E-MSTK-01", "Dashboard"] },
    { text: "Dashboard is the home page that every employee sees first.", size: 11 },
  ]);
  assert.equal(
    md,
    "## 1. Document Author\n\n| 1 | Hartono | Project Sponsor |\n| 3 | Willy Project Manager |\n\n### 3.2 Appendix\n\n| E-MSTK-01 | Dashboard |\nDashboard is the home page that every employee sees first.\n",
  );
});

// Stored rows of cells have no delimiter row, so the viewer saw one long line.
test("rows of cells become tables for display", () => {
  assert.equal(asTables("Intro\n\n| ID | Menu |\n| PAY-1 | Slip |\n\nAfter"), "Intro\n\n| ID | Menu |\n| --- | --- |\n| PAY-1 | Slip |\n\nAfter");
  // A wider row widens the first, so no cell is dropped.
  assert.equal(asTables("| 1 | Hartono |\n| 3 | Willy | PM |"), "| 1 | Hartono | |\n| --- | --- | --- |\n| 3 | Willy | PM |");
  const done = "| a | b |\n| --- | --- |\n| c | d |";
  assert.equal(asTables(done), done);
  const code = "```\n| not | a table |\n```";
  assert.equal(asTables(code), code);
});

test("the first heading titles the document", () => {
  assert.equal(firstHeading("Intro text\n\n# **Arunika DMS — Spesifikasi** #\n\n## 1. Penjualan\n"), "Arunika DMS — Spesifikasi");
  assert.equal(firstHeading("#hashtag is not a heading\n### Payslip\n"), "Payslip");
  assert.equal(firstHeading("No headings here."), "");
});
