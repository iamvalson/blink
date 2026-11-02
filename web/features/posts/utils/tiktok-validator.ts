export interface TikTokCreatorInfo {
 creator_avatar_url: string;
 creator_username: string;
 creator_nickname: string;
 privacy_level_options: string[];
 comment_disabled: boolean;
 duet_disabled: boolean;
 stitch_disabled: boolean;
 max_video_post_duration_sec: number;
}
export interface TikTokOptions {
 privacy_level: string;
 disable_comment: boolean;
 disable_duet: boolean;
 disable_stitch: boolean;
 commercial_content: boolean;
 brand_content_toggle: boolean;
 brand_organic_toggle: boolean;
 is_aigc: boolean;
 consent: boolean;
 duration_seconds: number;
}
export const emptyTikTokOptions: TikTokOptions = {
 privacy_level: "", disable_comment: true, disable_duet: true, disable_stitch: true,
 commercial_content: false, brand_content_toggle: false, brand_organic_toggle: false,
 is_aigc: false, consent: false, duration_seconds: 0,
};
export function effectiveTikTokOptions(options: TikTokOptions, creator?: TikTokCreatorInfo): TikTokOptions {
 return { ...options,
  disable_comment: options.disable_comment || Boolean(creator?.comment_disabled),
  disable_duet: options.disable_duet || Boolean(creator?.duet_disabled),
  disable_stitch: options.disable_stitch || Boolean(creator?.stitch_disabled),
 };
}
export function validateTikTokPost(
 options: TikTokOptions, creator: TikTokCreatorInfo | undefined,
 media: { contentType: string; complete: boolean; size: number }[],
 metadata: {duration: number; width: number; height: number} | null,
 caption: string,
): string | null {
 if (!creator) return "Load current TikTok creator settings before publishing.";
 if (media.length !== 1 || !media[0].complete || !["video/mp4", "video/quicktime", "video/webm"].includes(media[0].contentType)) return "TikTok requires exactly one completed video upload.";
 if (!metadata || !Number.isFinite(metadata.duration) || metadata.duration <= 0 || metadata.duration > creator.max_video_post_duration_sec || metadata.duration > 600) return "Video duration exceeds the TikTok creator limit or could not be read.";
 if (metadata.width < 360 || metadata.width > 4096 || metadata.height < 360 || metadata.height > 4096) return "TikTok video dimensions must be between 360 and 4096 pixels.";
 if (media[0].size <= 0 || media[0].size > 4 * 1024 ** 3) return "Video size is outside TikTok's supported range.";
 if (caption.length > 2200) return "TikTok captions may contain at most 2200 UTF-16 characters.";
 if (!creator.privacy_level_options.includes(options.privacy_level)) return "Select an available TikTok privacy setting.";
 if (options.brand_content_toggle && !["PUBLIC_TO_EVERYONE", "MUTUAL_FOLLOW_FRIENDS"].includes(options.privacy_level)) return "Branded content cannot be private or followers-only.";
 if (options.commercial_content && !options.brand_content_toggle && !options.brand_organic_toggle) return "Choose Your brand, Branded content, or both.";
 if (!options.consent) return "Confirm TikTok's policies and authorize publishing this video.";
 return null;
}
