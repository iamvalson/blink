import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import { createRequire } from "node:module";
import ts from "typescript";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";

const require = createRequire(import.meta.url);
function load(path, overrides) {
  const { outputText } = ts.transpileModule(fs.readFileSync(new URL(path, import.meta.url), "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 },
  });
  const exports = {};
  vm.runInThisContext(`(function(require,exports){${outputText}\n})`)(name => name in overrides ? overrides[name] : require(name), exports);
  return exports;
}
const Link = ({ children, ...props }) => React.createElement("a", props, children);
const Button = ({ children, ...props }) => React.createElement("button", Object.fromEntries(Object.entries(props).filter(([key]) => !["variant", "size"].includes(key))), children);
const buttonVariants = () => "button";
const StatusBadge = ({ status }) => React.createElement("span", null, status);
const FormatBadge = ({ format }) => React.createElement("span", null, format);
const shared = {
  "next/link": { default: Link },
  "@/components/ui/button": { Button, buttonVariants },
  "@/components/posts/status-badge": { StatusBadge, FormatBadge },
};
const post = {
  id: "post-123", caption: "A post with full details", title: "Launch update", status: "SCHEDULED", content_format: "VIDEO",
  created_at: "2026-11-01T10:00:00Z", updated_at: "2026-11-02T10:00:00Z", scheduled_at: "2026-11-10T10:00:00Z",
  targets: [{ id: "target-1", platform: "youtube", platform_user_id: "creator", status: "PENDING" }],
};
const ready = data => ({ data, isPending: false, isError: false, refetch() {} });
let mode = "ready";
let calls = [];
const overview = load("./features/posts/components/overview.tsx", {
  ...shared,
  "@/features/posts/hooks/use-posts": {
    usePosts(filter) {
      calls.push(filter);
      if (mode === "loading") return { isPending: true };
      if (mode === "error") return { isError: true };
      return ready({ posts: mode === "empty" ? [] : [post], total: mode === "empty" ? 0 : filter.status ? 73 : 240 });
    },
    useUpcomingPosts: () => mode === "error" ? { isError: true } : mode === "loading" ? { isPending: true } : ready({ posts: mode === "empty" ? [] : [post] }),
  },
  "@/features/accounts/hooks/use-accounts": {
    useAccounts: () => mode === "error" ? { isError: true } : mode === "loading" ? { isPending: true } : ready({ accounts: mode === "empty" ? [] : [{ id: "account-1", platform: "youtube", platform_user_id: "creator", reconnect_required: true }] }),
  },
});
const render = Component => renderToStaticMarkup(React.createElement(Component));
let html = render(overview.Overview);
assert.match(html, /240/); // Uses the server total, not the six-row activity page.
assert.match(html, /73/);
assert.match(html, /href="\/posts\?status=PUBLISHED"/);
assert.match(html, /href="\/posts\/post-123"/);
assert.match(html, /href="\/calendar"/);
assert.match(html, /Reconnect/);
assert.deepEqual(calls.map(c => c.status).filter(Boolean), ["PUBLISHED", "SCHEDULED", "FAILED"]);
mode = "empty";
html = render(overview.Overview);
assert.match(html, /Create your first post/);
assert.match(html, /Connect your first account/);
assert.match(html, /No posts scheduled/);
mode = "loading";
assert.match(render(overview.Overview), /Loading recent posts/);
mode = "error";
html = render(overview.Overview);
assert.match(html, /Could not load recent posts/);
assert.match(html, /Could not load scheduled posts/);
assert.match(html, /Could not load accounts/);

const cards = Object.fromEntries(["Card", "CardContent", "CardHeader"].map(name => [name, ({ children, ...props }) => React.createElement("div", props, children)]));
const previewComponent = load("./components/posts/post-preview.tsx", { "@/components/ui/card": cards });
let mediaQuery = ready({ content_type: "video/mp4", upload_status: "complete" });
const previewModule = load("./features/posts/components/post-details-preview.tsx", {
  "@tanstack/react-query": { useQuery: () => mediaQuery },
  "@/lib/api/client": { apiClient() {} },
  "@/components/posts/post-preview": previewComponent,
});
const renderPreview = value => renderToStaticMarkup(React.createElement(previewModule.PostDetailsPreview, { post: value }));
let previewHtml = renderPreview({ ...post, media_id: "uploaded-media" });
assert.match(previewHtml, /Post preview/);
assert.match(previewHtml, /creator/);
assert.match(previewHtml, /uploaded-media\/content/);
assert.match(previewHtml, /<video[^>]*controls/);
mediaQuery = ready({ content_type: "image/png", upload_status: "complete" });
previewHtml = renderPreview({ ...post, media_id: "uploaded-image" });
assert.match(previewHtml, /<img/);
assert.match(previewHtml, /uploaded-image\/content/);
assert.doesNotMatch(renderPreview(post), /<video|<img|Your post caption will appear/);
previewHtml = renderPreview({ ...post, targets: [...post.targets, { id: "target-2", platform: "twitter", platform_user_id: "another-creator" }] });
assert.match(previewHtml, /aria-label="Preview platform"/);
assert.match(previewHtml, /aria-pressed="true"/);
mediaQuery = { isPending: true };
assert.match(renderPreview({ ...post, media_id: "loading" }), /Loading attached media/);
mediaQuery = { isError: true };
assert.match(renderPreview({ ...post, media_id: "unavailable" }), /Retry media/);
mediaQuery = ready({ content_type: "image/png", upload_status: "pending" });
assert.match(renderPreview({ ...post, media_id: "pending" }), /not available for preview/);

let query = ready(post);
let mutation;
const requests = [];
const invalidations = [];
const navigation = [];
class ApiError extends Error { constructor(statusCode) { super("API failed"); this.statusCode = statusCode; } }
const details = load("./features/posts/components/post-details.tsx", {
  ...shared,
  react: { ...React, useState: value => [value, () => {}] },
  "next/navigation": { useRouter: () => ({ replace: path => navigation.push(path) }) },
  "@tanstack/react-query": {
    useMutation(options) { mutation = options; return { mutate() {}, isPending: false }; },
    useQueryClient: () => ({ invalidateQueries: async options => invalidations.push(options.queryKey), setQueryData() {}, removeQueries() {} }),
  },
  "@/features/posts/hooks/use-posts": { usePost: id => { assert.equal(id, post.id); return query; } },
  "@/features/posts/api/create-post": Object.fromEntries(["retryPost", "cancelSchedule", "deletePost"].map(name => [name, async id => { requests.push([name, id]); return post; }])),
  "@/lib/api/client": { ApiError },
  "./post-details-preview": previewModule,
  "@/components/posts/publishing-progress": { PublishingProgress: ({ post, onRetry }) => React.createElement("div", null, post.status, onRetry ? React.createElement("button", null, "Retry publish") : null) },
  "@/components/posts/reschedule-post-dialog": { ReschedulePostDialog: () => null },
});
function renderDetails() { return renderToStaticMarkup(details.PostDetails({ id: post.id })); }
html = renderDetails();
assert.match(html, /A post with full details/);
assert.match(html, /Launch update/);
assert.match(html, /Reschedule/);
assert.match(html, /Cancel schedule/);
assert.match(html, /Post ID/);
for (const [status, retry, canDelete] of [["FAILED", true, true], ["PARTIALLY_PUBLISHED", true, true], ["PUBLISHING", false, false], ["PUBLISHED", false, true]]) {
  query = ready({ ...post, status });
  html = renderDetails();
  assert.equal(html.includes("Retry publish"), retry);
  assert.equal(html.includes(">Delete<"), canDelete);
  assert.equal(html.includes(">Reschedule<"), false);
}
query = ready({ ...post, media_url: "https://example.com/video.mp4", media_type: "video/mp4" });
assert.match(renderDetails(), /<video[^>]*controls/);
query = ready({ ...post, targets: [{ ...post.targets[0], error_code: "ACCOUNT_RECONNECT_REQUIRED" }] });
assert.match(renderDetails(), /Reconnect account/);
query = { isPending: true };
assert.match(renderDetails(), /Loading post details/);
query = { isError: true, error: new ApiError(404) };
assert.match(renderDetails(), /Post not found/);
query = { isError: true, error: new ApiError(500) };
assert.match(renderDetails(), /Try again/);
for (const [kind, api] of [["retry", "retryPost"], ["cancel", "cancelSchedule"], ["delete", "deletePost"]]) {
  await mutation.mutationFn(kind);
  assert.deepEqual(requests.at(-1), [api, post.id]);
  await mutation.onSuccess(kind === "delete" ? undefined : post, kind);
}
assert.ok(invalidations.some(key => key[0] === "posts"));
assert.ok(invalidations.some(key => key[0] === "calendar"));
assert.deepEqual(navigation, ["/posts"]);

const page = load("./app/(dashboard)/posts/[id]/page.tsx", { "@/features/posts/components/post-details": { PostDetails: details.PostDetails } });
assert.equal((await page.default({ params: Promise.resolve({ id: post.id }) })).props.id, post.id);
const postsPage = load("./app/(dashboard)/posts/page.tsx", { ...shared, "@/features/posts/components/posts-list": { PostsList: ({ initialStatus }) => React.createElement("p", null, initialStatus ?? "all") } });
assert.match(renderToStaticMarkup(await postsPage.default({ searchParams: Promise.resolve({ status: "FAILED" }) })), /FAILED/);
assert.match(renderToStaticMarkup(await postsPage.default({ searchParams: Promise.resolve({ status: "invalid" }) })), />all</);
console.log("Overview and post details: counts, links, states, action permissions, API mutations, invalidation, and route parameters passed.");

const hooks = load("./features/posts/hooks/use-posts.ts", {
  "@tanstack/react-query": { useQuery: options => options },
  "../api/create-post": {},
  "./use-post-status-stream": { usePostStatusStream() {} },
});
const options = hooks.usePost(post.id);
for (const status of ["PUBLISHED", "PARTIALLY_PUBLISHED", "FAILED", "CANCELLED"]) {
  assert.equal(options.refetchInterval({ state: { data: { status } } }), false);
}
assert.equal(options.refetchInterval({ state: { data: { status: "SCHEDULED" } } }), 30000);
assert.equal(options.refetchInterval({ state: { data: { status: "QUEUED" } } }), 5000);

const originalEventSource = globalThis.EventSource;
const originalApiUrl = process.env.NEXT_PUBLIC_API_URL;
process.env.NEXT_PUBLIC_API_URL = "https://api.example.com";
const eventSources = [];
let closed = false;
const refreshed = [];
globalThis.EventSource = class {
  handlers = {};
  constructor() { eventSources.push(this); }
  addEventListener(name, handler) { this.handlers[name] = handler; }
  close() { closed = true; }
};
try {
  const streams = load("./features/posts/hooks/use-post-status-stream.ts", {
    react: { useEffect: effect => effect(), useRef: () => ({ current: null }) },
    "@tanstack/react-query": { useQueryClient: () => ({ setQueryData() {}, invalidateQueries: options => refreshed.push(options.queryKey) }) },
    "./use-posts": { postQueryKey: id => ["posts", id] },
  });
  streams.usePostStatusStream(post.id);
  eventSources.at(-1).handlers.status({ data: JSON.stringify({ ...post, status: "PUBLISHED" }) });
  assert.ok(closed);
  assert.deepEqual(refreshed, [["posts"], ["calendar"]]);
} finally {
  globalThis.EventSource = originalEventSource;
  if (originalApiUrl === undefined) delete process.env.NEXT_PUBLIC_API_URL;
  else process.env.NEXT_PUBLIC_API_URL = originalApiUrl;
}
console.log("Live status updates refresh all post and calendar queries; active retries retain polling fallback.");
