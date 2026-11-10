import assert from "node:assert/strict";
import { feedPhotoDimensions, prepareInstagramFeedPhoto } from "./features/posts/utils/instagram-feed-photo.ts";

assert.deepEqual(feedPhotoDimensions(2160, 2700), { width: 1080, height: 1350, needsFraming: false });
assert.deepEqual(feedPhotoDimensions(200, 200), { width: 320, height: 320, needsFraming: false });
assert.deepEqual(feedPhotoDimensions(1080, 1920), { width: 1080, height: 1350, needsFraming: true });
assert.deepEqual(feedPhotoDimensions(2000, 500), { width: 1080, height: 566, needsFraming: true });

const drawings = [];
let closed = false;
globalThis.createImageBitmap = async (_, options) => {
  assert.equal(options.imageOrientation, "from-image");
  return { width: 1080, height: 1920, close: () => { closed = true; } };
};
globalThis.document = {
  createElement() {
    return {
      getContext: () => ({ fillRect() {}, drawImage: (_, ...box) => drawings.push(box) }),
      toBlob: (callback, type) => callback(new Blob(["encoded"], { type })),
    };
  },
};
const source = new File(["source"], "photo.webp", { type: "image/webp" });
const result = await prepareInstagramFeedPhoto(source);
assert.equal(result.automatic, undefined);
assert.equal(result.fit.type, "image/jpeg");
assert.equal(result.crop.type, "image/jpeg");
assert.equal(source.type, "image/webp");
assert.deepEqual(drawings, [[160.3125, 0, 759.375, 1350], [0, -285, 1080, 1920]]);
assert.equal(closed, true);
console.log("Instagram feed photo framing and JPEG preparation checks passed.");

drawings.length = 0;
const carousel = await prepareInstagramFeedPhoto(source, "CAROUSEL");
assert.equal(carousel.width, 1080);
assert.equal(carousel.height, 1080);
assert.equal(carousel.fit.type, "image/jpeg");
assert.equal(carousel.crop.type, "image/jpeg");
assert.deepEqual(drawings[1], [0, -420, 1080, 1920]);

const story = await prepareInstagramFeedPhoto(source, "STORIES");
assert.equal(story.width, 1080);
assert.equal(story.height, 1920);
assert.equal(story.automatic.type, "image/jpeg");
console.log("Instagram carousel and Story photo geometry checks passed.");
