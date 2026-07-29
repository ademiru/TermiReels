import assert from "node:assert/strict";
import test from "node:test";
import { decodedRequestText, extractMedia, targetUserID } from "./media.js";
import { creatorReelCode, selectTargetUserID } from "./collector.js";

test("extractMedia preserves array order and playable payload", () => {
  const value = {
    data: {
      edges: [
        { node: { pk: "1", code: "first", user: { username: "a" }, video_versions: [{ url: "one" }] } },
        { node: { pk: "2", code: "second", user: { username: "b" }, video_versions: [{ url: "two" }] } },
      ],
    },
  };
  assert.deepEqual(extractMedia(value).map((item) => item.code), ["first", "second"]);
  assert.equal(extractMedia(value)[0]?.videoURL, "one");
});

test("extractMedia accepts GraphQL shortcode and numeric identity variants", () => {
  const value = {
    data: {
      media: {
        id: 123,
        shortcode: "graphql-code",
        owner: { username: "creator" },
        video_resources: [{ src: "small" }, { src: "large" }],
      },
    },
  };
  assert.deepEqual(
    extractMedia(value).map(({ code, pk, owner, videoURL }) => ({ code, pk, owner, videoURL })),
    [{ code: "graphql-code", pk: "123", owner: "creator", videoURL: "large" }],
  );
});

test("extractMedia keeps the richest duplicate payload without changing order", () => {
  const value = {
    summary: { pk: "1", code: "same" },
    full: {
      pk: "1",
      code: "same",
      user: { username: "creator" },
      video_versions: [{ url: "https://video.example/reel.mp4" }],
    },
  };
  const media = extractMedia(value);
  assert.equal(media.length, 1);
  assert.equal(media[0]?.videoURL, "https://video.example/reel.mp4");
  assert.equal(media[0]?.owner, "creator");
});

test("targetUserID reads decoded GraphQL variables", () => {
  const text = decodedRequestText(
    "https://www.instagram.com/graphql/query?variables=%7B%22target_user_id%22%3A%2212345%22%7D",
    null,
  );
  assert.equal(targetUserID(text), "12345");
  assert.equal(targetUserID("https://www.instagram.com/api/v1/feed/user/98765/?count=12"), "98765");
});

test("selectTargetUserID chooses the response correlated with the grid", () => {
  const media = (code: string) => extractMedia({ code, pk: code })[0]!;
  assert.equal(selectTargetUserID([
    { targetID: "wrong", media: [media("suggestion")] },
    { targetID: "right", media: [media("grid-a"), media("grid-b")] },
  ], ["grid-a", "grid-b"]), "right");
});

test("creatorReelCode accepts only creator-scoped grid links", () => {
  assert.equal(creatorReelCode("/creator/reel/ABC_123/", "/creator/reels/"), "ABC_123");
  assert.equal(creatorReelCode("/someone/reel/ABC_123/", "/creator/reels/"), "");
  assert.equal(creatorReelCode("/reel/ABC_123/", "/creator/reels/"), "");
});
