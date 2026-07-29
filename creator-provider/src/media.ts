export type MediaCandidate = {
  code: string;
  pk: string;
  owner: string;
  videoURL: string;
  profilePicURL: string;
  caption: string;
  liked: boolean;
  saved: boolean;
  reposted: boolean;
  likeCount: number;
  commentCount: number;
  repostCount: number;
  commentsDisabled: boolean;
  verified: boolean;
  canViewerReshare: boolean;
  musicTitle: string;
  musicArtist: string;
  musicExplicit: boolean;
};

export function extractMedia(root: unknown): MediaCandidate[] {
  const output: MediaCandidate[] = [];
  const indexes = new Map<string, number>();
  const walk = (value: unknown): void => {
    if (Array.isArray(value)) {
      for (const child of value) walk(child);
      return;
    }
    if (!value || typeof value !== "object") return;
    const node = value as Record<string, unknown>;
    const code = stringValue(node.code) || stringValue(node.shortcode);
    const pk = idValue(node.pk) || idValue(node.id);
    const versions = Array.isArray(node.video_versions) ? node.video_versions : [];
    const first = versions[0] as Record<string, unknown> | undefined;
    const resources = Array.isArray(node.video_resources) ? node.video_resources : [];
    const resource = resources.at(-1) as Record<string, unknown> | undefined;
    const videoURL = stringValue(first?.url) || stringValue(node.video_url) || stringValue(resource?.src);
    const user = (node.user ?? node.owner) as Record<string, unknown> | undefined;
    const owner = stringValue(user?.username);
    const captionNode = node.caption as Record<string, unknown> | undefined;
    const clipsMetadata = node.clips_metadata as Record<string, unknown> | undefined;
    const musicInfo = clipsMetadata?.music_info as Record<string, unknown> | undefined;
    const asset = musicInfo?.music_asset_info as Record<string, unknown> | undefined;
    if (code && pk) {
      const candidate = {
        code,
        pk,
        owner,
        videoURL,
        profilePicURL: stringValue(user?.profile_pic_url),
        caption: stringValue(captionNode?.text),
        liked: booleanValue(node.has_liked),
        saved: booleanValue(node.has_viewer_saved),
        reposted: booleanValue(node.has_viewer_reposted),
        likeCount: numberValue(node.like_count),
        commentCount: numberValue(node.comment_count),
        repostCount: numberValue(node.media_repost_count),
        commentsDisabled: booleanValue(node.comments_disabled),
        verified: booleanValue(user?.is_verified),
        canViewerReshare: booleanValue(node.can_viewer_reshare),
        musicTitle: stringValue(asset?.title),
        musicArtist: stringValue(asset?.display_artist),
        musicExplicit: booleanValue(asset?.is_explicit),
      };
      const index = indexes.get(code);
      if (index === undefined) {
        indexes.set(code, output.length);
        output.push(candidate);
      } else if (mediaScore(candidate) > mediaScore(output[index]!)) {
        output[index] = candidate;
      }
    }
    for (const child of Object.values(node)) walk(child);
  };
  walk(root);
  return output;
}

function mediaScore(media: MediaCandidate): number {
  return (media.videoURL !== "" ? 8 : 0) +
    (media.owner !== "" ? 4 : 0) +
    (media.profilePicURL !== "" ? 2 : 0) +
    (media.caption !== "" ? 1 : 0);
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function idValue(value: unknown): string {
  if (typeof value === "string") return value;
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? String(value) : "";
}

function booleanValue(value: unknown): boolean {
  return value === true;
}

function numberValue(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? Math.trunc(value) : 0;
}

export function decodedRequestText(url: string, postData: string | null): string {
  const raw = `${url} ${postData ?? ""}`.toLowerCase();
  try {
    return decodeURIComponent(raw);
  } catch {
    return raw;
  }
}

export function targetUserID(requestText: string): string {
  const patterns = [
    /"target_user_id"\s*:\s*"(\d+)"/,
    /target_user_id(?:%22)?(?:=|%3a)\s*(?:%22)?(\d+)/,
    /"targetuserid"\s*:\s*"(\d+)"/,
    /\/api\/v1\/feed\/user\/(\d+)\//,
  ];
  for (const pattern of patterns) {
    const match = requestText.match(pattern);
    if (match?.[1]) return match[1];
  }
  return "";
}
