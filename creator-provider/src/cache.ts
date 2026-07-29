import { randomUUID } from "node:crypto";
import type { CreatorSnapshot } from "./protocol.js";

type CacheEntry = {
  storedAt: number;
  snapshot: CreatorSnapshot;
};

// SnapshotCache keeps only fully verified immutable pages. A cache hit gets a
// fresh source identity so asynchronous work from an earlier UI visit cannot
// be mistaken for the new visit.
export class SnapshotCache {
  private readonly entries = new Map<string, CacheEntry>();

  constructor(private readonly ttlMs: number) {}

  put(snapshot: CreatorSnapshot, now = Date.now()): void {
    if (snapshot.items.length === 0) return;
    this.entries.set(snapshot.username, {
      storedAt: now,
      snapshot: structuredClone(snapshot),
    });
  }

  get(username: string, now = Date.now()): CreatorSnapshot | undefined {
    const entry = this.entries.get(username);
    if (!entry) return undefined;
    if (now-entry.storedAt > this.ttlMs) {
      this.entries.delete(username);
      return undefined;
    }
    const snapshot = structuredClone(entry.snapshot);
    snapshot.source_id = randomUUID();
    return snapshot;
  }
}
