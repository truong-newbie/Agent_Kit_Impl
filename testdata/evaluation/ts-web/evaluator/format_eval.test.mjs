import test from "node:test";
import assert from "node:assert/strict";
import { formatCents, escapeHTML } from "../src/format.mjs";

test("formats negative cents properly and rejects NaN", () => {
  assert.equal(formatCents(-105), "-$1.05");
  assert.throws(() => formatCents(Number.NaN));
});

test("escapes ampersands and quotes", () => {
  assert.equal(escapeHTML('a & "b"'), "a &amp; &quot;b&quot;");
});
