import { mkdir } from "node:fs/promises";
import path from "node:path";
import { intToRGBA, Jimp } from "jimp";

const root = path.resolve(import.meta.dirname, "..");
const outputDirectory = path.join(root, "src/assets/hero/kutub-cutouts");
const sources = [1, 2, 5, 6];
const skySampleStep = 8;
const skySampleHeight = 0.62;
const skyRangePadding = 18;
const hardSkyDistance = 42;
const softSkyDistance = 8;
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

  for (let x = 0; x < image.width; x += skySampleStep) sample(x, 0);
  for (let y = 0; y < image.height * skySampleHeight; y += skySampleStep) {
    sample(0, y);
    sample(image.width - 1, y);
  }

  const channelRange = (channel) => {
    const values = samples.map((value) => value[channel]);
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

function largestOpaqueComponent(image, sky, number) {
  const width = image.width;
  const height = image.height;
  const neighbors = [-1, 0, 1];
  const opaque = new Uint8Array(width * height);
  const alpha = new Uint8Array(width * height);

  image.scan((x, y, index) => {
    const pixel = y * width + x;
    const distance = rangeDistance(
      image.bitmap.data[index],
      image.bitmap.data[index + 1],
      image.bitmap.data[index + 2],
      sky,
    );
    const warmStone = image.bitmap.data[index] > image.bitmap.data[index + 2] + 8;
    const value = Math.max(
      0,
      Math.min(255, ((distance - softSkyDistance) / (hardSkyDistance - softSkyDistance)) * 255),
    );
    alpha[pixel] = value;
    const belowTower = number === 6 && y > height * 0.93;
    const outsideTowerColumn = number === 6 && (x < width * 0.25 || x > width * 0.75);
    opaque[pixel] = value === 255 && warmStone && !belowTower && !outsideTowerColumn ? 1 : 0;
  });

  // Close small gaps in the warm stone mask so tower bands remain one component.
  let connected = opaque;
  for (let pass = 0; pass < 2; pass += 1) {
    const expanded = connected.slice();
    for (let y = 0; y < height; y += 1) {
      for (let x = 0; x < width; x += 1) {
        const pixel = y * width + x;
        if (connected[pixel]) continue;
        for (const offsetY of neighbors) {
          for (const offsetX of neighbors) {
            const nextX = x + offsetX;
            const nextY = y + offsetY;
            if (
              nextX >= 0 &&
              nextX < width &&
              nextY >= 0 &&
              nextY < height &&
              connected[nextY * width + nextX]
            ) {
              expanded[pixel] = 1;
            }
          }
        }
      }
    }
    connected = expanded;
  }

  const visited = new Uint8Array(width * height);
  let largest = [];
  let largestInterior = [];
  let largestBottom = [];
  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const start = y * width + x;
      if (!connected[start] || visited[start]) continue;
    const component = [];
    const queue = [start];
    let touchesSkyBorder = false;
    let touchesBottom = false;
    visited[start] = 1;
      for (let cursor = 0; cursor < queue.length; cursor += 1) {
        const current = queue[cursor];
        component.push(current);
        const currentX = current % width;
        const currentY = Math.floor(current / width);
        if (currentX === 0 || currentX === width - 1 || currentY === 0) {
          touchesSkyBorder = true;
        }
        if (currentY === height - 1) touchesBottom = true;
        for (const offsetY of neighbors) {
          for (const offsetX of neighbors) {
            if (offsetX === 0 && offsetY === 0) continue;
            const nextX = currentX + offsetX;
            const nextY = currentY + offsetY;
            if (nextX < 0 || nextX >= width || nextY < 0 || nextY >= height) continue;
            const next = nextY * width + nextX;
            if (connected[next] && !visited[next]) {
              visited[next] = 1;
              queue.push(next);
            }
          }
        }
      }
      if (component.length > largest.length) largest = component;
      if (!touchesSkyBorder && component.length > largestInterior.length) {
        largestInterior = component;
      }
      if (touchesBottom && !touchesSkyBorder && component.length > largestBottom.length) {
        largestBottom = component;
      }
    }
  }

  const kept = new Uint8Array(width * height);
  const selected = largestBottom.length
    ? largestBottom
    : largestInterior.length
      ? largestInterior
      : largest;
  for (const pixel of selected) kept[pixel] = 1;
  const featherNeighbors = [[-1, 0], [1, 0], [0, -1], [0, 1]];
  image.scan((x, y, index) => {
    const pixel = y * width + x;
    let outputAlpha = kept[pixel] ? 255 : 0;
    if (!outputAlpha) {
      for (const [offsetX, offsetY] of featherNeighbors) {
        const nextX = x + offsetX;
        const nextY = y + offsetY;
        if (
          alpha[pixel] >= 200 &&
          nextX >= 0 &&
          nextX < width &&
          nextY >= 0 &&
          nextY < height &&
          kept[nextY * width + nextX]
        ) {
          outputAlpha = alpha[pixel];
        }
      }
    }
    image.bitmap.data[index + 3] = outputAlpha;
  });

  return { componentPixels: selected.length };
}

async function processSource(number) {
  const input = path.join(root, `src/assets/hero/kutub-source/source-${number}.jpg`);
  const output = path.join(outputDirectory, `cutout-${number}-color.png`);
  const image = await Jimp.read(input);
  const sourceSize = `${image.width}x${image.height}`;
  const sky = sampleSky(image);
  const component = largestOpaqueComponent(image, sky, number);
  const scale = outputLongEdge / Math.max(image.width, image.height);
  image.resize({ w: Math.round(image.width * scale), h: Math.round(image.height * scale) });
  image.rotate(90);
  await image.write(output);
  console.log(JSON.stringify({ number, sky, sourceSize, componentPixels: component.componentPixels, output }));
}

await mkdir(outputDirectory, { recursive: true });
await Promise.all(sources.map(processSource));
