import assert from "node:assert/strict";
import test from "node:test";
import { linesToMarkdown } from "./convert.ts";

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
