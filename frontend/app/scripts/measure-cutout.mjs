import path from "node:path";
import { intToRGBA, Jimp } from "jimp";

const file = process.argv[2] ?? "src/assets/hero/kutub-cutouts/cutout-2.png";
const image = await Jimp.read(path.resolve(file));
const { width, height } = image;
const opaque = new Uint8Array(width * height);
let partial = 0;

for (let y = 0; y < height; y += 1) {
  for (let x = 0; x < width; x += 1) {
    const alpha = intToRGBA(image.getPixelColor(x, y)).a;
    opaque[y * width + x] = alpha === 255 ? 1 : 0;
    if (alpha > 0 && alpha < 255) partial += 1;
  }
}

const visited = new Uint8Array(opaque.length);
const queue = new Int32Array(opaque.length);
const components = [];
let boundingBox = null;

for (let start = 0; start < opaque.length; start += 1) {
  if (!opaque[start] || visited[start]) continue;
  let queueStart = 0;
  let queueEnd = 1;
  let size = 0;
  let minX = width;
  let minY = height;
  let maxX = -1;
  let maxY = -1;
  queue[0] = start;
  visited[start] = 1;

  while (queueStart < queueEnd) {
    const index = queue[queueStart++];
    const x = index % width;
    const y = Math.floor(index / width);
    size += 1;
    minX = Math.min(minX, x);
    minY = Math.min(minY, y);
    maxX = Math.max(maxX, x);
    maxY = Math.max(maxY, y);

    for (let offsetY = -1; offsetY <= 1; offsetY += 1) {
      for (let offsetX = -1; offsetX <= 1; offsetX += 1) {
        if (offsetX === 0 && offsetY === 0) continue;
        const neighborX = x + offsetX;
        const neighborY = y + offsetY;
        if (neighborX < 0 || neighborX >= width || neighborY < 0 || neighborY >= height) continue;
        const neighbor = neighborY * width + neighborX;
        if (opaque[neighbor] && !visited[neighbor]) {
          visited[neighbor] = 1;
          queue[queueEnd++] = neighbor;
        }
      }
    }
  }

  components.push({ size, minX, minY, maxX, maxY });
}

components.sort((a, b) => b.size - a.size);
if (components[0]) {
  boundingBox = [components[0].minX, components[0].minY, components[0].maxX, components[0].maxY];
}

console.log(JSON.stringify({
  file,
  dimensions: `${width}x${height}`,
  opaqueBoundingBox: boundingBox,
  connectedOpaqueComponents: components.length,
  largestComponentPixels: components[0]?.size ?? 0,
  strayOpaquePixels: components.slice(1).reduce((sum, component) => sum + component.size, 0),
  partialAlphaPixels: partial,
  partialAlphaPercent: Number((partial / (width * height) * 100).toFixed(2)),
}, null, 2));
