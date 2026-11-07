"use client";
import type { TikTokCreatorInfo, TikTokOptions } from "@/features/posts/utils/tiktok-validator";
interface Props {
 creator?: TikTokCreatorInfo;
 options: TikTokOptions;
 onChange: (options: TikTokOptions) => void;
 disabled?: boolean;
 loading?: boolean;
 error?: string | null;
 onRefresh: () => void;
}
export function TikTokSettings({ creator, options, onChange, disabled, loading, error, onRefresh }: Props) {
 const change = (patch: Partial<TikTokOptions>) => onChange({ ...options, consent: false, ...patch });
 return <fieldset disabled={disabled} className="min-w-0 space-y-4 rounded-lg border p-3 text-sm wrap-anywhere sm:p-4">
  <legend className="px-1 font-medium">TikTok publishing settings</legend>
  {loading ? <p role="status">Loading TikTok creator settings…</p> : creator ? <p>Publishing to <strong>{creator.creator_nickname}</strong> (@{creator.creator_username}). Maximum video length: {creator.max_video_post_duration_sec} seconds.</p> : <p role="alert">Creator settings could not be loaded.</p>}
  <button type="button" onClick={onRefresh} disabled={loading} className="text-primary underline">Refresh creator settings</button>
  <label className="block space-y-1">Privacy
   <select aria-label="TikTok privacy" value={options.privacy_level} onChange={e => change({privacy_level:e.target.value})} className="block min-h-11 w-full min-w-0 rounded border bg-background p-2 text-base sm:text-sm">
    <option value="" disabled>Select privacy</option>
    {creator?.privacy_level_options.map(level => <option key={level} value={level} disabled={!["PUBLIC_TO_EVERYONE", "MUTUAL_FOLLOW_FRIENDS"].includes(level) && options.brand_content_toggle}>{({PUBLIC_TO_EVERYONE:"Everyone",MUTUAL_FOLLOW_FRIENDS:"Friends",FOLLOWER_OF_CREATOR:"Followers",SELF_ONLY:"Only me"} as Record<string,string>)[level] ?? level}</option>)}
   </select>
  </label>
  <div className="flex flex-wrap gap-4">
   {([ ["Comment", "disable_comment", creator?.comment_disabled], ["Duet", "disable_duet", creator?.duet_disabled], ["Stitch", "disable_stitch", creator?.stitch_disabled] ] as const).map(([label,key,unavailable]) => <label key={key} className={unavailable ? "text-muted-foreground" : ""}><input type="checkbox" checked={!options[key] && !unavailable} disabled={!creator || unavailable} onChange={e => change({[key]:!e.target.checked})} /> Allow {label}</label>)}
  </div>
  <label className="block"><input type="checkbox" checked={options.commercial_content} onChange={e => change({commercial_content:e.target.checked,brand_content_toggle:false,brand_organic_toggle:false})} /> Disclose commercial content</label>
  {options.commercial_content && <div className="space-y-2 rounded border p-3">
   <label className="block"><input type="checkbox" checked={options.brand_organic_toggle} onChange={e => change({brand_organic_toggle:e.target.checked})} /> Your brand</label>
   <label className="block" title={["SELF_ONLY", "FOLLOWER_OF_CREATOR"].includes(options.privacy_level) ? "Branded content cannot be private" : undefined}><input type="checkbox" checked={options.brand_content_toggle} disabled={["SELF_ONLY", "FOLLOWER_OF_CREATOR"].includes(options.privacy_level)} onChange={e => change({brand_content_toggle:e.target.checked})} /> Branded content</label>
   {(options.brand_content_toggle || options.brand_organic_toggle) && <p>Your video will be labeled as {options.brand_content_toggle ? "Paid partnership" : "Promotional content"}.</p>}
  </div>}
  <label className="block"><input type="checkbox" checked={options.is_aigc} onChange={e => change({is_aigc:e.target.checked})} /> This video is AI generated</label>
  <label className="block"><input type="checkbox" checked={options.consent} onChange={e => change({consent:e.target.checked})} /> I authorize Blink to publish this video to the account above, now or at my selected scheduled time.</label>
  <p>By posting, you agree to TikTok&apos;s {options.brand_content_toggle && <><a className="underline" href="https://www.tiktok.com/legal/page/global/bc-policy/en" target="_blank" rel="noreferrer">Branded Content Policy</a> and </>}<a className="underline" href="https://www.tiktok.com/legal/page/global/music-usage-confirmation/en" target="_blank" rel="noreferrer">Music Usage Confirmation</a>.</p>
  <p className="text-muted-foreground">Review the video preview before publishing. TikTok may take several minutes to process your video. Scheduled posts use these choices and recheck account permissions when publishing.</p>
  {error && <p role="alert" className="text-destructive">{error}</p>}
 </fieldset>;
}
