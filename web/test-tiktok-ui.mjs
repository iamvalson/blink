import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import { createRequire } from "node:module";
import ts from "typescript";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
const validationSource=fs.readFileSync(new URL("./features/posts/utils/tiktok-validator.ts",import.meta.url),"utf8");
const validationJS=ts.transpileModule(validationSource,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText;
const validationExports={};
vm.runInThisContext(`(function(require,exports){${validationJS}\n})`)(createRequire(import.meta.url),validationExports);
const {emptyTikTokOptions}=validationExports;
// Render the production component. Type-only imports are erased by TypeScript.
const source = fs.readFileSync(new URL("./components/posts/tiktok-settings.tsx", import.meta.url), "utf8");
const {outputText} = ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,jsx:ts.JsxEmit.ReactJSX,target:ts.ScriptTarget.ES2022}});
const exports = {};
vm.runInThisContext(`(function(require,exports){${outputText}\n})`)(createRequire(import.meta.url),exports);
const creator={creator_username:"tester",creator_nickname:"Test Creator",privacy_level_options:["SELF_ONLY","PUBLIC_TO_EVERYONE"],max_video_post_duration_sec:60,comment_disabled:true,duet_disabled:false,stitch_disabled:false};
const props={creator,options:emptyTikTokOptions,onChange(){},onRefresh(){}};
const html=renderToStaticMarkup(React.createElement(exports.TikTokSettings,props));
assert.match(html,/Test Creator/);
assert.match(html,/<option value="" disabled="" selected="">Select privacy/);
assert.doesNotMatch(html,/checked=""/);
assert.match(html,/disabled=""[^>]*\/> Allow Comment/);
assert.match(html,/Music Usage Confirmation/);
const branded=renderToStaticMarkup(React.createElement(exports.TikTokSettings,{...props,options:{...emptyTikTokOptions,commercial_content:true,brand_content_toggle:true,privacy_level:"PUBLIC_TO_EVERYONE"}}));
assert.match(branded,/Branded Content Policy/);
assert.match(branded,/Paid partnership/);
const failed=renderToStaticMarkup(React.createElement(exports.TikTokSettings,{...props,error:"Privacy is no longer available"}));
assert.match(failed,/role="alert"[^>]*>Privacy is no longer available/);
console.log("TikTok settings UI: 8 rendered component checks passed.");

const progressSource=fs.readFileSync(new URL("./components/posts/publishing-progress.tsx",import.meta.url),"utf8");
const progressJS=ts.transpileModule(progressSource,{compilerOptions:{module:ts.ModuleKind.CommonJS,jsx:ts.JsxEmit.ReactJSX,target:ts.ScriptTarget.ES2022}}).outputText;
const require=createRequire(import.meta.url);const progressExports={};
const progressRequire=name=>{
 if(name==="@/lib/utils")return {cn:(...values)=>values.filter(Boolean).join(" ")};
 if(name==="@/components/ui/button")return {Button:({children,onClick,disabled})=>React.createElement("button",{onClick,disabled},children),buttonVariants:()=>""};
 if(name==="next/link")return {__esModule:true,default:({children,href})=>React.createElement("a",{href},children)};
 return require(name);
};
vm.runInThisContext(`(function(require,exports){${progressJS}\n})`)(progressRequire,progressExports);
const renderProgress=status=>renderToStaticMarkup(React.createElement(progressExports.PublishingProgress,{post:{id:"post-id",status:"PUBLISHING",targets:[{id:"target-id",platform:"tiktok",status,error_message:"Review TikTok before retrying"}]}}));
assert.match(renderProgress("PROCESSING"),/Publication is awaiting confirmation/);
assert.match(renderProgress("UNKNOWN"),/Needs review/);
assert.match(renderProgress("UNKNOWN"),/Review TikTok before retrying/);
const published=renderToStaticMarkup(React.createElement(progressExports.PublishingProgress,{post:{id:"post-id",status:"PUBLISHED",targets:[{id:"target-id",platform:"tiktok",status:"PUBLISHED"}]}}));
assert.match(published,/Published/);assert.doesNotMatch(published,/View on TikTok/);
console.log("TikTok publication UI: 5 rendered status checks passed.");

const retrying=renderToStaticMarkup(React.createElement(progressExports.PublishingProgress,{post:{id:"post-id",status:"PUBLISHING",targets:[{id:"target-id",platform:"tiktok",status:"RETRYING",error_message:"worker requires ffprobe for TikTok video validation"}]}}));
assert.match(retrying,/TikTok retry scheduled:/);
assert.match(retrying,/worker requires ffprobe/);
console.log("Retry diagnostics UI: 2 rendered checks passed.");

const inbox = renderToStaticMarkup(React.createElement(progressExports.PublishingProgress, {
  post: { id: "post-id", status: "INBOX_DELIVERED", targets: [{ id: "target-id", platform: "tiktok", status: "INBOX_DELIVERED" }] },
}));
assert.match(inbox, /Sent to TikTok Inbox/);
assert.match(inbox, /Open the notification in your TikTok Inbox/);
assert.doesNotMatch(inbox, />Published</);
