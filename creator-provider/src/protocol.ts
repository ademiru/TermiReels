export const PROTOCOL_VERSION = 1;

export type RequestFrame = {
  version: number;
  id: string;
  method: "health" | "warm" | "creator.resolve" | "creator.cancel";
  params?: Record<string, unknown>;
};

export type ProviderItem = {
  ordinal: number;
  shortcode: string;
  pk: string;
  owner_username: string;
  video_url: string;
  profile_pic_url: string;
  caption: string;
  liked: boolean;
  saved: boolean;
  reposted: boolean;
  like_count: number;
  comment_count: number;
  repost_count: number;
  comments_disabled: boolean;
  verified: boolean;
  can_viewer_reshare: boolean;
  music_title: string;
  music_artist: string;
  music_explicit: boolean;
  grid_seen: true;
  target_response_seen: true;
};

export type CreatorSnapshot = {
  source_id: string;
  username: string;
  instagram_user_id: string;
  revision: 1;
  items: ProviderItem[];
  next_cursor: string;
};

export type ResponseFrame =
  | { version: 1; id: string; result: unknown }
  | { version: 1; id: string; error: { code: string; message: string } };

export function parseRequest(line: string): RequestFrame {
  const value: unknown = JSON.parse(line);
  if (!value || typeof value !== "object") throw new Error("request must be an object");
  const frame = value as Partial<RequestFrame>;
  if (frame.version !== PROTOCOL_VERSION) throw new Error("unsupported protocol version");
  if (typeof frame.id !== "string" || frame.id.length === 0) throw new Error("missing request id");
  if (!["health", "warm", "creator.resolve", "creator.cancel"].includes(frame.method ?? "")) {
    throw new Error("unknown method");
  }
  return frame as RequestFrame;
}

export function success(id: string, result: unknown): ResponseFrame {
  return { version: 1, id, result };
}

export function failure(id: string, code: string, error: unknown): ResponseFrame {
  const message = error instanceof Error ? error.message : String(error);
  return { version: 1, id, error: { code, message } };
}
