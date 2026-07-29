import { randomUUID } from "node:crypto";
import { chromium, type Browser, type BrowserContext, type Page, type Response } from "playwright-core";
import { decodedRequestText, extractMedia, targetUserID, type MediaCandidate } from "./media.js";
import type { CreatorSnapshot, ProviderItem } from "./protocol.js";

export type TargetBatch = { targetID: string; media: MediaCandidate[] };
type CaptureAudit = {
  inspected: number;
  mediaResponses: number;
  targetResponses: number;
  unscopedRoutes: Set<string>;
};

export class CreatorCollector {
  private browser: Browser | undefined;
  private connecting: Promise<Browser> | undefined;

  constructor(private readonly cdpEndpoint: string) {}

  async warm(): Promise<void> {
    await this.context();
  }

  private async context(): Promise<BrowserContext> {
    if (!this.browser) {
      this.connecting ??= chromium.connectOverCDP(this.cdpEndpoint);
      try {
        this.browser = await this.connecting;
      } finally {
        this.connecting = undefined;
      }
    }
    const context = this.browser.contexts()[0];
    if (!context) throw new Error("authenticated Chromium context is unavailable");
    return context;
  }

  async close(): Promise<void> {
    // Playwright has no public "disconnect only" operation for CDP browsers.
    // Browser.close() would own the shared Chromium lifetime, so deliberately
    // release only our reference. Process exit closes the CDP transport.
    this.browser = undefined;
    this.connecting = undefined;
  }

  async resolve(usernameInput: string, limit: number, signal: AbortSignal): Promise<CreatorSnapshot> {
    const username = normalizeUsername(usernameInput);
    const context = await this.context();
    const page = await context.newPage();
    const batches: TargetBatch[] = [];
    const audit: CaptureAudit = {
      inspected: 0,
      mediaResponses: 0,
      targetResponses: 0,
      unscopedRoutes: new Set(),
    };
    const pendingResponses = new Set<Promise<void>>();
    const onResponse = (response: Response): void => {
      const pending = captureTargetBatch(response, batches, audit);
      pendingResponses.add(pending);
      void pending.finally(() => pendingResponses.delete(pending));
    };
    page.on("response", onResponse);
    const abort = (): void => { void page.close().catch(() => undefined); };
    signal.addEventListener("abort", abort, { once: true });

    try {
      await page.goto(`https://www.instagram.com/${encodeURIComponent(username)}/reels/`, {
        waitUntil: "domcontentloaded",
        timeout: 15_000,
      });
      await waitForProfileRoute(page, username, signal);
      const gridCodes = await collectGridCodes(page, username, limit, signal);
      await page.waitForTimeout(350);
      await Promise.allSettled([...pendingResponses]);
      let userID: string;
      try {
        userID = selectTargetUserID(batches, gridCodes);
      } catch (error) {
        throw new Error(`${error instanceof Error ? error.message : String(error)}; ${auditSummary(audit, batches, gridCodes)}`);
      }

      const apiCodes = new Set(
        batches.filter((batch) => batch.targetID === userID)
          .flatMap((batch) => batch.media.map((media) => media.code)),
      );
      const intersection = gridCodes.filter((code) => apiCodes.has(code)).slice(0, limit);
      if (intersection.length === 0) {
        throw new Error("profile grid and target-user response have no verified intersection");
      }

      const resolvedByCode = new Map<string, MediaCandidate>();
      for (const batch of batches) {
        if (batch.targetID !== userID) continue;
        for (const media of batch.media) {
          const previous = resolvedByCode.get(media.code);
          if (isPlayable(media) && (!previous || !isPlayable(previous))) {
            resolvedByCode.set(media.code, media);
          }
        }
      }
      const unresolvedCodes = intersection.filter((code) => !resolvedByCode.has(code));
      const scopedByCode = new Map<string, MediaCandidate>();
      for (const batch of batches) {
        if (batch.targetID !== userID) continue;
        for (const media of batch.media) {
          const previous = scopedByCode.get(media.code);
          if (!previous || mediaCompleteness(media) > mediaCompleteness(previous)) {
            scopedByCode.set(media.code, media);
          }
        }
      }
      const infoResolved = await resolveMediaInfo(
        context,
        unresolvedCodes.map((code) => scopedByCode.get(code)).filter((media): media is MediaCandidate => Boolean(media)),
        signal,
      );
      for (const media of infoResolved) resolvedByCode.set(media.code, media);
      const directFallback = unresolvedCodes.filter((code) => !resolvedByCode.has(code));
      process.stderr.write(
        `creator resolve @${username}: grid=${gridCodes.length} intersected=${intersection.length} ` +
        `scoped_playable=${intersection.length - unresolvedCodes.length} ` +
        `media_info=${infoResolved.length} direct_fallback=${directFallback.length}\n`,
      );
      for (const media of await resolveInOrder(context, directFallback, scopedByCode, signal)) {
        resolvedByCode.set(media.code, media);
      }
      const resolved = intersection
        .map((code) => resolvedByCode.get(code))
        .filter((media): media is MediaCandidate => Boolean(media));
      const items: ProviderItem[] = resolved.map((media, index) => ({
        ordinal: index + 1,
        shortcode: media.code,
        pk: media.pk,
        owner_username: media.owner,
        video_url: media.videoURL,
        profile_pic_url: media.profilePicURL,
        caption: media.caption,
        liked: media.liked,
        saved: media.saved,
        reposted: media.reposted,
        like_count: media.likeCount,
        comment_count: media.commentCount,
        repost_count: media.repostCount,
        comments_disabled: media.commentsDisabled,
        verified: media.verified,
        can_viewer_reshare: media.canViewerReshare,
        music_title: media.musicTitle,
        music_artist: media.musicArtist,
        music_explicit: media.musicExplicit,
        grid_seen: true,
        target_response_seen: true,
      }));
      if (items.length === 0) throw new Error("no intersected reel produced a playable payload");
      return {
        source_id: randomUUID(),
        username,
        instagram_user_id: userID,
        revision: 1,
        items,
        next_cursor: "",
      };
    } finally {
      signal.removeEventListener("abort", abort);
      page.off("response", onResponse);
      await page.close().catch(() => undefined);
    }
  }
}

async function captureTargetBatch(
  response: Response,
  batches: TargetBatch[],
  audit: CaptureAudit,
): Promise<void> {
  const request = response.request();
  if (!["xhr", "fetch"].includes(request.resourceType())) return;
  const route = safeRoute(request.url());
  if (!route.includes("graphql") && !route.includes("/api/")) return;
  audit.inspected++;
  const text = decodedRequestText(request.url(), request.postData());
  const targetID = targetUserID(text);
  try {
    const media = extractMedia(await response.json());
    if (media.length === 0) return;
    audit.mediaResponses++;
    if (!targetID) {
      audit.unscopedRoutes.add(route);
      return;
    }
    audit.targetResponses++;
    batches.push({ targetID, media });
  } catch {
    // Non-JSON and expired response bodies are irrelevant.
  }
}

function auditSummary(audit: CaptureAudit, batches: TargetBatch[], gridCodes: string[]): string {
  const targets = [...new Set(batches.map((batch) => batch.targetID))].slice(0, 6);
  const scopedCodes = new Set(batches.flatMap((batch) => batch.media.map((media) => media.code)));
  const overlap = gridCodes.filter((code) => scopedCodes.has(code)).length;
  return [
    `grid=${gridCodes.length}`,
    `xhr=${audit.inspected}`,
    `media_responses=${audit.mediaResponses}`,
    `target_responses=${audit.targetResponses}`,
    `targets=${targets.join(",") || "none"}`,
    `overlap=${overlap}`,
    `unscoped=${[...audit.unscopedRoutes].slice(0, 4).join(",") || "none"}`,
  ].join(" ");
}

function safeRoute(value: string): string {
  try {
    return new URL(value).pathname;
  } catch {
    return "invalid-url";
  }
}

async function waitForProfileRoute(page: Page, username: string, signal: AbortSignal): Promise<void> {
  const expected = `/${username.toLowerCase()}/reels/`;
  let stable = 0;
  for (let attempts = 0; stable < 3 && attempts < 50; attempts++) {
    if (signal.aborted) throw new Error("creator resolution canceled");
    const state = await page.evaluate((path) => {
      const normalize = (value: string) => `${value.toLowerCase().replace(/\/+$/, "")}/`;
      const main = document.querySelector("main");
      const tab = main && [...main.querySelectorAll<HTMLAnchorElement>('a[href]')]
        .some((anchor) => normalize(new URL(anchor.href, location.origin).pathname) === path);
      return { path: normalize(location.pathname), tab: Boolean(tab) };
    }, expected);
    stable = state.path === expected && state.tab ? stable + 1 : 0;
    await page.waitForTimeout(120);
  }
  if (stable < 3) throw new Error(`profile reels route did not settle at ${expected}`);
}

export function selectTargetUserID(batches: TargetBatch[], gridCodes: string[]): string {
  const grid = new Set(gridCodes);
  const scores = new Map<string, number>();
  for (const batch of batches) {
    let score = scores.get(batch.targetID) ?? 0;
    for (const media of batch.media) {
      if (grid.has(media.code)) score++;
    }
    scores.set(batch.targetID, score);
  }
  let selected = "";
  let highest = 0;
  for (const [targetID, score] of scores) {
    if (score > highest) {
      selected = targetID;
      highest = score;
    }
  }
  if (!selected || highest === 0) throw new Error("numeric profile identity could not be correlated with the visible grid");
  return selected;
}

async function collectGridCodes(page: Page, username: string, limit: number, signal: AbortSignal): Promise<string[]> {
  const output: string[] = [];
  const seen = new Set<string>();
  const expected = `/${username.toLowerCase()}/reels/`;
  for (let stable = 0, attempts = 0; attempts < 24 && output.length < limit; attempts++) {
    if (signal.aborted) throw new Error("creator resolution canceled");
    const paths = await page.evaluate((path) => {
      const normalize = (value: string) => `${value.toLowerCase().replace(/\/+$/, "")}/`;
      if (normalize(location.pathname) !== path) return [];
      const main = document.querySelector("main");
      if (!main) return [];
      const tab = [...main.querySelectorAll<HTMLAnchorElement>('a[href]')]
        .find((anchor) => normalize(new URL(anchor.href, location.origin).pathname) === path);
      const tabBottom = tab?.getBoundingClientRect().bottom ?? 0;
      const creatorPrefix = path.replace(/\/reels\/$/, "/reel/");
      return [...main.querySelectorAll<HTMLAnchorElement>('a[href]')]
        .filter((anchor) => {
          const box = anchor.getBoundingClientRect();
          const pathname = new URL(anchor.href, location.origin).pathname;
          return pathname.startsWith(creatorPrefix) &&
            box.top >= tabBottom - 2 &&
            box.width > 40 &&
            box.height > 40;
        })
        .map((anchor) => new URL(anchor.href, location.origin).pathname);
    }, expected);
    const codes = paths
      .map((pathname) => creatorReelCode(pathname, expected))
      .filter((code) => code !== "");
    const before = output.length;
    for (const code of codes) {
      if (!seen.has(code)) {
        seen.add(code);
        output.push(code);
      }
    }
    stable = output.length === before ? stable + 1 : 0;
    if (stable >= 4 && output.length > 0) break;
    await page.evaluate(() => window.scrollBy(0, Math.max(innerHeight * 1.4, 600)));
    await page.waitForTimeout(180);
  }
  return output;
}

export function creatorReelCode(pathname: string, profileReelsPath: string): string {
  const prefix = profileReelsPath.toLowerCase().replace(/\/reels\/?$/, "/reel/");
  if (!pathname.toLowerCase().startsWith(prefix)) return "";
  const suffix = pathname.slice(prefix.length);
  const code = suffix.split("/")[0] ?? "";
  return /^[A-Za-z0-9_-]+$/.test(code) ? code : "";
}

async function resolveInOrder(
  context: BrowserContext,
  codes: string[],
  scopedByCode: Map<string, MediaCandidate>,
  signal: AbortSignal,
): Promise<MediaCandidate[]> {
  const resolved = new Array<MediaCandidate | undefined>(codes.length);
  let nextIndex = 0;
  const worker = async (): Promise<void> => {
    while (true) {
      const index = nextIndex++;
      if (index >= codes.length) return;
      if (signal.aborted) throw new Error("creator resolution canceled");
      const code = codes[index];
      if (!code) continue;
      resolved[index] = await resolveOne(context, code, scopedByCode.get(code), signal).catch(() => undefined);
    }
  };
  await Promise.all(Array.from({ length: Math.min(12, codes.length) }, worker));
  return resolved.filter((media): media is MediaCandidate => Boolean(media));
}

function isPlayable(media: MediaCandidate): boolean {
  return media.pk !== "" && media.videoURL.startsWith("https://");
}

function mediaCompleteness(media: MediaCandidate): number {
  return (isPlayable(media) ? 8 : 0) +
    (media.owner !== "" ? 4 : 0) +
    (media.profilePicURL !== "" ? 2 : 0) +
    (media.caption !== "" ? 1 : 0);
}

async function resolveMediaInfo(
  context: BrowserContext,
  seeds: MediaCandidate[],
  signal: AbortSignal,
): Promise<MediaCandidate[]> {
  const results = await Promise.all(seeds.map(async (seed) => {
    if (signal.aborted) return undefined;
    try {
      const response = await context.request.get(
        `https://www.instagram.com/api/v1/media/${encodeURIComponent(seed.pk)}/info/`,
        {
          headers: {
            "x-ig-app-id": "936619743392459",
            "x-requested-with": "XMLHttpRequest",
          },
          timeout: 5_000,
        },
      );
      if (!response.ok()) return undefined;
      const match = extractMedia(await response.json())
        .find((media) => media.code === seed.code && media.pk === seed.pk && isPlayable(media));
      return match ? mergeMedia(seed, match) : undefined;
    } catch {
      return undefined;
    }
  }));
  return results.filter((media): media is MediaCandidate => Boolean(media));
}

function mergeMedia(seed: MediaCandidate, resolved: MediaCandidate): MediaCandidate {
  return {
    ...resolved,
    owner: resolved.owner || seed.owner,
    profilePicURL: resolved.profilePicURL || seed.profilePicURL,
    caption: resolved.caption || seed.caption,
    musicTitle: resolved.musicTitle || seed.musicTitle,
    musicArtist: resolved.musicArtist || seed.musicArtist,
  };
}

async function resolveOne(
  context: BrowserContext,
  code: string,
  seed: MediaCandidate | undefined,
  signal: AbortSignal,
): Promise<MediaCandidate | undefined> {
  const page = await context.newPage();
  let finish: ((value: MediaCandidate | undefined) => void) | undefined;
  const found = new Promise<MediaCandidate | undefined>((resolve) => { finish = resolve; });
  const onResponse = (response: Response): void => {
    void (async () => {
      try {
        const match = extractMedia(await response.json())
          .find((media) => media.code === code && media.videoURL !== "");
        if (match) finish?.(seed ? mergeMedia(seed, match) : match);
      } catch {
        // Ignore non-media responses.
      }
    })();
  };
  page.on("response", onResponse);
  const abort = (): void => finish?.(undefined);
  signal.addEventListener("abort", abort, { once: true });
  try {
    await page.goto(`https://www.instagram.com/reel/${encodeURIComponent(code)}/`, {
      waitUntil: "domcontentloaded",
      timeout: 12_000,
    });
    return await Promise.race([
      found,
      new Promise<undefined>((resolve) => setTimeout(() => resolve(undefined), 4_000)),
    ]);
  } finally {
    signal.removeEventListener("abort", abort);
    page.off("response", onResponse);
    await page.close().catch(() => undefined);
  }
}

function normalizeUsername(value: string): string {
  const username = value.trim().replace(/^@/, "").toLowerCase();
  if (!/^[a-z0-9._]{1,30}$/.test(username)) throw new Error("invalid Instagram username");
  return username;
}
