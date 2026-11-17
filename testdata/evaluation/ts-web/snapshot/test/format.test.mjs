import test from "node:test";
import assert from "node:assert/strict";
import { formatCents } from "../src/format.mjs";

test("formats positive cents", () => {
  assert.equal(formatCents(1250), "$12.50");
});
