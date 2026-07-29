import { createInterface } from "node:readline";
import { SnapshotCache } from "./cache.js";
import { CreatorCollector } from "./collector.js";
import { failure, parseRequest, success, type RequestFrame, type ResponseFrame } from "./protocol.js";

const endpoint = process.env.TERMIREELS_CDP_ENDPOINT ?? "http://127.0.0.1:6767";
const collector = new CreatorCollector(endpoint);
const snapshots = new SnapshotCache(60_000);
const active = new Map<string, AbortController>();
const input = createInterface({ input: process.stdin, crlfDelay: Infinity });

function write(frame: ResponseFrame): void {
  process.stdout.write(`${JSON.stringify(frame)}\n`);
}

async function handle(frame: RequestFrame): Promise<void> {
  if (frame.method === "health") {
    write(success(frame.id, { status: "ok", protocol: 1 }));
    return;
  }
  if (frame.method === "warm") {
    try {
      await collector.warm();
      write(success(frame.id, { status: "ready" }));
    } catch (error) {
      write(failure(frame.id, "warm_failed", error));
    }
    return;
  }
  if (frame.method === "creator.cancel") {
    const target = typeof frame.params?.request_id === "string" ? frame.params.request_id : "";
    active.get(target)?.abort();
    write(success(frame.id, { canceled: Boolean(target) }));
    return;
  }
  const username = typeof frame.params?.username === "string" ? frame.params.username : "";
  const requestedLimit = typeof frame.params?.limit === "number" ? frame.params.limit : 12;
  const limit = Math.max(1, Math.min(Math.trunc(requestedLimit), 24));
  const controller = new AbortController();
  active.set(frame.id, controller);
  try {
    const normalized = username.trim().replace(/^@/, "").toLowerCase();
    const cached = snapshots.get(normalized);
    if (cached) {
      write(success(frame.id, cached));
      return;
    }
    const snapshot = await collector.resolve(username, limit, controller.signal);
    snapshots.put(snapshot);
    if (controller.signal.aborted) {
      write(failure(frame.id, "canceled", new Error("creator resolution canceled")));
    } else {
      write(success(frame.id, snapshot));
    }
  } catch (error) {
    write(failure(frame.id, controller.signal.aborted ? "canceled" : "resolve_failed", error));
  } finally {
    active.delete(frame.id);
  }
}

input.on("line", (line) => {
  if (!line.trim()) return;
  let frame: RequestFrame;
  try {
    frame = parseRequest(line);
  } catch (error) {
    write(failure("", "invalid_request", error));
    return;
  }
  void handle(frame);
});

async function shutdown(): Promise<void> {
  for (const controller of active.values()) controller.abort();
  await collector.close();
  process.exit(0);
}

process.once("SIGINT", () => { void shutdown(); });
process.once("SIGTERM", () => { void shutdown(); });
