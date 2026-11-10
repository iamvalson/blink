import assert from "node:assert/strict";
import fs from "node:fs";
import { createRequire } from "node:module";
import ts from "typescript";

const source = fs.readFileSync(new URL("./components/posts/media-uploader.tsx", import.meta.url), "utf8");
const { outputText } = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 },
});
const require = createRequire(import.meta.url);
const exports = {};
new Function("require", "exports", outputText)(
  name => name === "react" ? { useRef: () => ({ current: null }) } : require(name),
  exports,
);

function findInput(element) {
  if (!element || typeof element !== "object") return;
  if (Array.isArray(element)) return element.map(findInput).find(Boolean);
  if (element.type === "input") return element;
  return findInput(element.props?.children);
}

const files = [new File(["one"], "one.mp4"), new File(["two"], "two.mp4")];
const accept = "video/mp4,video/quicktime,.mp4,.mov";
for (const maxFiles of [1, undefined]) {
  let attachments;
  const uploader = exports.MediaUploader({ media: [], maxFiles, accept, onChange: items => { attachments = items; } });
  const input = findInput(uploader);
  assert.equal(input.props.multiple, maxFiles !== 1);
  assert.equal(input.props.accept, accept);
  input.props.onChange({ target: { files } });
  assert.equal(attachments.length, maxFiles ?? 2);
  assert.equal(attachments[0].file.name, "one.mp4");
  for (const { previewUrl } of attachments) URL.revokeObjectURL(previewUrl);
  if (maxFiles === 1) {
    assert.equal(findInput(exports.MediaUploader({ media: attachments, maxFiles, onChange() {} })), undefined);
  }
}
console.log("Media uploader respects single-file selection and keeps multiple uploads available in other modes.");
