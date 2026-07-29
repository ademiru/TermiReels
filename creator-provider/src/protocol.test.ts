import assert from "node:assert/strict";
import test from "node:test";
import { parseRequest } from "./protocol.js";

test("parseRequest accepts versioned resolve requests", () => {
  const frame = parseRequest('{"version":1,"id":"x","method":"creator.resolve","params":{"username":"a"}}');
  assert.equal(frame.id, "x");
});

test("parseRequest accepts provider warm-up requests", () => {
  const frame = parseRequest('{"version":1,"id":"warm","method":"warm"}');
  assert.equal(frame.method, "warm");
});

test("parseRequest rejects version drift", () => {
  assert.throws(() => parseRequest('{"version":2,"id":"x","method":"health"}'));
});
