# Phase P9.27: Qutub Minar Cutouts

## Approach

`npm install --no-save jimp` succeeded, so the Jimp path was used. The standalone
script is `app/scripts/cutout-kutub.mjs`. It decodes each JPEG, samples four upper
corner sky pixels, walks every pixel, removes pixels matching the sampled sky or
cloud-like sky colors, applies a 3px silhouette feather, desaturates the tower
with luminance conversion, crops tightly around the tower, and writes PNGs with
RGBA alpha.

Sky-distance threshold: `110` RGB Euclidean units.

Sampled sky channel sums across four samples:

- source 3: RGB `(309, 531, 693)` total, average `(77.25, 132.75, 173.25)`
- source 6: RGB `(504, 660, 859)` total, average `(126, 165, 214.75)`

The per-source silhouette guides exclude source 3's trees and source 6's dome,
walls, and garden from the decorative tower assets. The tower's light central
bands are retained as the narrow tower core even when they resemble pale sky.

## Outputs

| File | Dimensions | Size | Alpha check |
| --- | ---: | ---: | --- |
| `app/src/assets/hero/kutub-cutouts/cutout-3.png` | 230 x 800 | 210,264 bytes | 92,625 visible pixels; 2,640 partially transparent |
| `app/src/assets/hero/kutub-cutouts/cutout-6.png` | 257 x 800 | 268,811 bytes | 103,646 visible pixels; 2,639 partially transparent |

Both files are non-empty grayscale PNGs with real transparent background and a
non-trivial alpha channel.
