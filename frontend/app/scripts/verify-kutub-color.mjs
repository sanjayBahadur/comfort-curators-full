import path from "node:path";
import { Jimp } from "jimp";

const directory = path.resolve(import.meta.dirname, "../src/assets/hero/kutub-cutouts");

for (const number of [1, 2, 5, 6]) {
  const image = await Jimp.read(path.join(directory, `cutout-${number}-color.png`));
  const { width, height, data } = image.bitmap;
  const alpha = new Uint8Array(width * height);
  let transparent = 0;
  let partial = 0;
  let opaque = 0;
  let minX = width;
  let minY = height;
  let maxX = -1;
  let maxY = -1;
  let colorOpaquePixels = 0;
  const colors = [];

  for (let pixel = 0; pixel < width * height; pixel += 1) {
    const index = pixel * 4;
    const value = data[index + 3];
    alpha[pixel] = value;
    if (value === 0) {
      transparent += 1;
      continue;
    }
    if (value === 255) opaque += 1;
    else partial += 1;
    const x = pixel % width;
    const y = Math.floor(pixel / width);
    minX = Math.min(minX, x);
    minY = Math.min(minY, y);
    maxX = Math.max(maxX, x);
    maxY = Math.max(maxY, y);
    if (value === 255 && (data[index] !== data[index + 1] || data[index] !== data[index + 2])) {
      colorOpaquePixels += 1;
      if (colors.length < 5) colors.push([data[index], data[index + 1], data[index + 2]]);
    }
  }

  const visited = new Uint8Array(width * height);
  let components = 0;
  let largest = 0;
  for (let start = 0; start < alpha.length; start += 1) {
    if (!alpha[start] || visited[start]) continue;
    components += 1;
    const queue = [start];
    visited[start] = 1;
    for (let cursor = 0; cursor < queue.length; cursor += 1) {
      const current = queue[cursor];
      const x = current % width;
      const y = Math.floor(current / width);
      for (let offsetY = -1; offsetY <= 1; offsetY += 1) {
        for (let offsetX = -1; offsetX <= 1; offsetX += 1) {
          if (!offsetX && !offsetY) continue;
          const nextX = x + offsetX;
          const nextY = y + offsetY;
          if (nextX < 0 || nextX >= width || nextY < 0 || nextY >= height) continue;
          const next = nextY * width + nextX;
          if (alpha[next] && !visited[next]) {
            visited[next] = 1;
            queue.push(next);
          }
        }
      }
    }
    largest = Math.max(largest, queue.length);
  }

  console.log(
    JSON.stringify({
      file: `cutout-${number}-color.png`,
      size: `${width}x${height}`,
      transparentFraction: (transparent / (width * height)).toFixed(4),
      partialFraction: (partial / (width * height)).toFixed(4),
      opaqueFraction: (opaque / (width * height)).toFixed(4),
      components,
      largest,
      bbox: [minX, minY, maxX, maxY],
      bboxFraction: (((maxX - minX + 1) * (maxY - minY + 1)) / (width * height)).toFixed(4),
      colorOpaquePixels,
      colors,
    }),
  );
}
