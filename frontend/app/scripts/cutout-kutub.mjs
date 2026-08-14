import { mkdir } from "node:fs/promises";
import path from "node:path";
import { intToRGBA, Jimp } from "jimp";

const root = path.resolve(import.meta.dirname, "..");
const outputDirectory = path.join(root, "src/assets/hero/kutub-cutouts");
const sources = process.argv.slice(2).map(Number).filter(Boolean);
const sourceNumbers = sources.length > 0 ? sources : [1, 4];

// These frames have a sky gradient. Sample the top edge and the upper side
// edges, stopping before the trees and ground enter the frame.
const skySampleStep = 4;
const skySampleHeight = 0.62;
const skyRangePadding = 28;
const hardSkyDistance = 28;
const edgeFeatherRadius = 1;
const outputLongEdge = 800;

function rangeDistance(red, green, blue, sky) {
  const outside = (value, range) => Math.max(range.minimum - value, 0, value - range.maximum);

  return Math.sqrt(
    outside(red, sky.red) ** 2 +
    outside(green, sky.green) ** 2 +
    outside(blue, sky.blue) ** 2,
  );
}

function sampleSky(image) {
  const samples = [];
  const sample = (x, y) => {
    const { r, g, b } = intToRGBA(image.getPixelColor(x, y));
    samples.push({ red: r, green: g, blue: b });
  };

  for (let x = 0; x < image.width; x += skySampleStep) {
    sample(x, 0);
    sample(x, Math.floor(image.height * 0.08));
    sample(x, Math.floor(image.height * 0.18));
  }
  for (let y = 0; y < image.height * skySampleHeight; y += skySampleStep) {
    sample(0, y);
    sample(image.width - 1, y);
  }

  const channelRange = (channel) => {
    const values = samples.map((sample) => sample[channel]);
    return {
      minimum: Math.max(0, Math.min(...values) - skyRangePadding),
      maximum: Math.min(255, Math.max(...values) + skyRangePadding),
    };
  };

  return {
    red: channelRange("red"),
    green: channelRange("green"),
    blue: channelRange("blue"),
    sampleCount: samples.length,
  };
}

function largestComponent(mask, width, height) {
  const component = new Uint8Array(mask.length);
  const queue = new Int32Array(mask.length);
  let largest = [];

  for (let start = 0; start < mask.length; start += 1) {
    if (!mask[start] || component[start]) continue;

    const pixels = [];
    let queueStart = 0;
    let queueEnd = 1;
    queue[0] = start;
    component[start] = 1;

    while (queueStart < queueEnd) {
      const index = queue[queueStart++];
      pixels.push(index);
      const x = index % width;
      const y = Math.floor(index / width);

      for (let offsetY = -1; offsetY <= 1; offsetY += 1) {
        for (let offsetX = -1; offsetX <= 1; offsetX += 1) {
          if (offsetX === 0 && offsetY === 0) continue;
          const neighborX = x + offsetX;
          const neighborY = y + offsetY;
          if (
            neighborX < 0 || neighborX >= width ||
            neighborY < 0 || neighborY >= height
          ) continue;
          const neighbor = neighborY * width + neighborX;
          if (mask[neighbor] && !component[neighbor]) {
            component[neighbor] = 1;
            queue[queueEnd++] = neighbor;
          }
        }
      }
    }

    if (pixels.length > largest.length) largest = pixels;
  }

  const result = new Uint8Array(mask.length);
  for (const index of largest) result[index] = 1;
  return result;
}

function featherMask(mask, width, height) {
  const alpha = new Uint8Array(mask.length);
  const radius = edgeFeatherRadius;

  for (let index = 0; index < mask.length; index += 1) {
    const x = index % width;
    const y = Math.floor(index / width);
    let nearest = radius + 1;

    for (let offsetY = -radius; offsetY <= radius; offsetY += 1) {
      for (let offsetX = -radius; offsetX <= radius; offsetX += 1) {
        const distance = Math.max(Math.abs(offsetX), Math.abs(offsetY));
        if (distance === 0 || distance >= nearest) continue;
        const neighborX = x + offsetX;
        const neighborY = y + offsetY;
        if (
          neighborX >= 0 && neighborX < width &&
          neighborY >= 0 && neighborY < height &&
          mask[neighborY * width + neighborX] !== mask[index]
        ) nearest = distance;
      }
    }

    if (mask[index]) {
      alpha[index] = nearest > radius ? 255 : Math.round((nearest / radius) * 255);
    } else if (nearest <= radius) {
      alpha[index] = Math.round(((radius - nearest + 1) / radius) * 127);
    }
  }

  return alpha;
}

async function processSource(number) {
  const input = path.join(root, `src/assets/hero/kutub-source/source-${number}.jpg`);
  const output = path.join(outputDirectory, `cutout-${number}.png`);
  const image = await Jimp.read(input);
  const scale = outputLongEdge / Math.max(image.width, image.height);
  image.resize({ w: Math.round(image.width * scale), h: Math.round(image.height * scale) });
  const sky = sampleSky(image);
  const mask = new Uint8Array(image.width * image.height);

  image.scan((x, y, index) => {
    const red = image.bitmap.data[index];
    const green = image.bitmap.data[index + 1];
    const blue = image.bitmap.data[index + 2];
    const distance = rangeDistance(red, green, blue, sky);
    mask[y * image.width + x] = distance > hardSkyDistance ? 1 : 0;
    const gray = Math.round(0.299 * red + 0.587 * green + 0.114 * blue);

    image.bitmap.data[index] = gray;
    image.bitmap.data[index + 1] = gray;
    image.bitmap.data[index + 2] = gray;
    image.bitmap.data[index + 3] = 0;
  });

  const towerMask = largestComponent(mask, image.width, image.height);
  const alpha = featherMask(towerMask, image.width, image.height);
  image.scan((x, y, index) => {
    image.bitmap.data[index + 3] = alpha[y * image.width + x];
  });

  await image.write(output);
  console.log(JSON.stringify({ number, sky, source: `${image.width}x${image.height}`, output }));
}

await mkdir(outputDirectory, { recursive: true });
await Promise.all(sourceNumbers.map(processSource));
