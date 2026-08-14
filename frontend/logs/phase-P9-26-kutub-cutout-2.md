# Phase P9.26: Qutub Cutouts

## Approach

Used the prescribed Jimp path. `npm install --no-save jimp` succeeded in the
`app/` workspace, so no rectangular fallback was needed. The standalone
script is `app/scripts/cutout-kutub.mjs`.

The script decodes each JPEG, averages 24x24 patches from all four corners to
sample the photo's sky, removes pixels by Euclidean RGB distance from that
sample, applies a soft alpha ramp at the silhouette edge, and desaturates all
remaining pixels to grayscale before writing PNG.

Parameters:

- Sky sample: four 24x24 corner patches
- Soft threshold start: RGB distance 38
- Fully opaque threshold: RGB distance 78
- Output long edge: 800px

## Results

| Source | Sampled sky RGB | Output | Dimensions | File size |
| --- | --- | --- | --- | ---: |
| `source-2.jpg` | `(183.10, 205.73, 241.54)` | `cutout-2.png` | 451x800 RGBA | 584 KB |
| `source-5.jpg` | `(178.86, 184.05, 189.13)` | `cutout-5.png` | 451x800 RGBA | 250 KB |

Both files contain alpha values from 0 through 255, substantial non-transparent
content, and equal RGB channels for every pixel.

## Verification

- `file`: both outputs report 8-bit/color RGBA PNG
- `npm run build`: passed
- `npm run lint`: passed with existing unrelated React warnings
