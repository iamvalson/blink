import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import { createRequire } from "node:module";
import ts from "typescript";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";

const require = createRequire(import.meta.url);
const cache = new Map();
function load(path, overrides = {}) {
  const source = fs.readFileSync(new URL(path, import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 },
  });
  const exports = {};
  const localRequire = (name) => {
    if (name in overrides) return overrides[name];
    if (cache.has(name)) return cache.get(name);
    return require(name);
  };
  vm.runInThisContext(`(function(require,exports){${outputText}\n})`)(localRequire, exports);
  return exports;
}

const schema = load("./features/auth/schemas/reset-password.ts");
const good = { password: "Replacement-password2!", confirm_password: "Replacement-password2!" };
assert.equal(schema.resetPasswordSchema.safeParse(good).success, true);
for (const password of ["short", "alllowercase1!", "ALLUPPERCASE1!", "NoNumbersHere!", "NoSymbolHere1", "Aa1 " + "a".repeat(8), "Aa1!" + "a".repeat(125)]) {
  assert.equal(schema.resetPasswordSchema.safeParse({ password, confirm_password: password }).success, false);
}
assert.equal(schema.resetPasswordSchema.safeParse({ ...good, confirm_password: "different" }).success, false);
assert.equal(schema.resetPasswordSchema.safeParse({ password: "Aa1!" + "😀".repeat(124), confirm_password: "Aa1!" + "😀".repeat(124) }).success, true);
const token = "A".repeat(43);
assert.equal(schema.readResetToken("#token=" + token), token);
for (const fragment of ["", "#token=short", "#other=" + token, "?token=" + token]) {
  assert.equal(schema.readResetToken(fragment), null);
}

const previousAPI = process.env.NEXT_PUBLIC_API_URL;
process.env.NEXT_PUBLIC_API_URL = "https://api.example.com";
const client = load("./lib/api/client.ts");
cache.set("@/lib/api/client", client);
const recovery = load("./features/auth/api/password-recovery.ts");
const previousFetch = globalThis.fetch;
const requests = [];
globalThis.fetch = async (url, options) => {
  requests.push({ url, options });
  return url.endsWith("/forgot-password")
    ? new Response(JSON.stringify({ message: "If an account exists..." }), { status: 202, headers: { "Content-Type": "application/json" } })
    : new Response(null, { status: 204 });
};
try {
  await recovery.requestPasswordReset("user@example.com");
  await recovery.resetPassword(token, good.password);
  assert.equal(requests[0].url, "https://api.example.com/auth/forgot-password");
  assert.deepEqual(JSON.parse(requests[0].options.body), { email: "user@example.com" });
  assert.equal(requests[1].url, "https://api.example.com/auth/reset-password");
  assert.deepEqual(JSON.parse(requests[1].options.body), { token, password: good.password });
  for (const { url, options } of requests) {
    assert.equal(options.method, "POST");
    assert.equal(options.credentials, "include");
    assert.equal(url.includes(token), false);
    assert.equal(url.includes(good.password), false);
  }
  globalThis.fetch = async () => new Response("too many attempts; try again later", { status: 429 });
  await assert.rejects(recovery.requestPasswordReset("user@example.com"), (error) => error.statusCode === 429 && error.message.includes("too many attempts"));

  const ui = {
    "@/components/ui/button": { Button: ({ children, ...props }) => React.createElement("button", props, children) },
    "@/components/ui/input": { Input: (props) => React.createElement("input", props) },
    "@/components/ui/label": { Label: ({ children, ...props }) => React.createElement("label", props, children) },
    "next/link": { __esModule: true, default: ({ children, ...props }) => React.createElement("a", props, children) },
    "../api/password-recovery": recovery,
    "../schemas/forgot-password": load("./features/auth/schemas/forgot-password.ts"),
    "../schemas/reset-password": schema,
    "@tanstack/react-query": { useQueryClient: () => ({ clear() {} }) },
  };
  const forgot = load("./features/auth/components/forgot-password-form.tsx", ui);
  const html = renderToStaticMarkup(React.createElement(forgot.ForgotPasswordForm));
  assert.match(html, /type="email"/);
  assert.match(html, /Send reset link/);
  assert.match(html, /href="\/login"/);
  const reset = load("./features/auth/components/reset-password-form.tsx", ui);
  assert.match(renderToStaticMarkup(React.createElement(reset.ResetPasswordForm)), /Opening reset link/);
} finally {
  globalThis.fetch = previousFetch;
  if (previousAPI === undefined) delete process.env.NEXT_PUBLIC_API_URL;
  else process.env.NEXT_PUBLIC_API_URL = previousAPI;
}
console.log("Password recovery schemas, fragment handling, API contracts, errors, and initial component rendering passed.");
