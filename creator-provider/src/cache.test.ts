import assert from "node:assert/strict";
import test from "node:test";
import { SnapshotCache } from "./cache.js";
import type { CreatorSnapshot } from "./protocol.js";

function snapshot(): CreatorSnapshot {
  return {
    source_id: "original",
    username: "creator",
    instagram_user_id: "123",
    revision: 1,
    items: [{
      ordinal: 1,
      shortcode: "code",
      pk: "456",
      owner_username: "creator",
      video_url: "https://video.example/reel.mp4",
      profile_pic_url: "",
      caption: "",
      liked: false,
      saved: false,
      reposted: false,
      like_count: 0,
      comment_count: 0,
      repost_count: 0,
      comments_disabled: false,
      verified: false,
      can_viewer_reshare: false,
      music_title: "",
      music_artist: "",
      music_explicit: false,
      grid_seen: true,
      target_response_seen: true,
    }],
    next_cursor: "",
  };
}

test("SnapshotCache returns an isolated source and expires entries", () => {
  const cache = new SnapshotCache(1_000);
  cache.put(snapshot(), 10);
  const hit = cache.get("creator", 20);
  assert.ok(hit);
  assert.notEqual(hit.source_id, "original");
  assert.equal(cache.get("creator", 1_011), undefined);
});
