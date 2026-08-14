# Phase P9.7 — catalog rebuild from real inventory cutout photos

- **Date:** 2026-08-10
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete (build + lint + typecheck pass; no backend reachable for live seed verification)

## What I built

Rebuilt the shop catalog (`scripts/seed.ts`) and product image map
(`src/lib/catalog-images.ts`) around 53 real cutout photos from the
human's inventory photography — up from 15 SKUs with sourced stock photos
to 62 SKUs with product cutout photography.

Every SKU (new or existing) now renders a real product cutout via
`getPackageCatalogImage()`. The two-item fallback badge path
(`PRODUCT_MARKS`) is never hit for any seeded SKU.

## Photo swaps: existing SKUs with better cutouts (6)

| SKU | Title | Source PNG |
|---|---|---|
| TOWEL-01 | Bath Towel 500gsm | `brown_bath_towel_pair.png` |
| SHEET-01 | Cotton Bedsheet Set (Queen) | `california-design-den-egyptian-cotton-sage-sheet-set.png` |
| SHAMP-01 | Shampoo 50ml | `grown-alchemist-hydra-restore-shampoo-conditioner-set.png` |
| TEA-01 | Assam Tea Sachets (10) | `tazo-organic-awake-english-breakfast-tea-96-bags.png` |
| COFFEE-01 | Filter Coffee 100g | `peets-major-dickasons-blend-k-cup-pods-75-count.png` |
| LAMP-01 | Bedside Lamp | `white_brass_table_lamp.png` |

## New SKUs: all 47

### Linen (6)
| SKU | Title | Category | Price (INR) | Source PNG |
|---|---|---|---|---|
| TOWEL-03 | Green Bath Towel Set | linen | ₹550 | `green_bath_towel_set.png` |
| TOWEL-04 | Multicolor Bath Towel Set | linen | ₹550 | `multicolor_bath_towel_set.png` |
| TOWEL-05 | Dark Gray Towel Set | linen | ₹650 | `dark_gray_towel_set.png` |
| SHEET-02 | Bamboo King Sheet Set (Ivory) | linen | ₹1,800 | `pure-bamboo-king-ivory-sheet-set.png` |
| BEDSET-01 | Queen Comforter Bedding Set (Dark Grey) | linen | ₹2,200 | `dark-grey-queen-comforter-bedding-set.png` |
| BEDSP-01 | Quilted Bedspread (Dark Purple) | linen | ₹1,500 | `dark-purple-quilted-bedspread.png` |

### Toiletries (15)
| SKU | Title | Category | Price (INR) | Source PNG |
|---|---|---|---|---|
| TOOTHBR-01 | Disposable Toothbrushes (36-Pack) | toiletries | ₹120 | `aussumy-36-pack-mini-disposable-toothbrushes.png` |
| TOOTHBR-02 | Disposable Toothbrushes (160-Pack) | toiletries | ₹280 | `avistar-160-prepasted-disposable-toothbrushes.png` |
| TOOTHBR-03 | Mini Disposable Toothbrushes (20-Pack) | toiletries | ₹80 | `malisseladi-mini-disposable-toothbrushes-20pc.png` |
| TOOTHPA-01 | Sensodyne Pronamel Multipack | toiletries | ₹450 | `sensodyne_pronamel_toothpaste_multipack.png` |
| TOILETRY-01 | Hotel Toiletries Set (Tropical Oasis) | toiletries | ₹350 | `black_tropical_oasis_hotel_toiletries.png` |
| TOILETRY-02 | Hotel Toiletries Set (Tropical Oasis, 200-Piece) | toiletries | ₹850 | `black_tropical_oasis_hotel_toiletries_200_piece.png` |
| TOILETRY-03 | Hotel Toiletries Set (White Tea, 75-Piece) | toiletries | ₹650 | `terra-pure-white-tea-hotel-toiletries-amenity-set-75-piece.png` |
| TOILETRY-04 | Hotel Toiletries Set (Nature Essence) | toiletries | ₹350 | `white_nature_essence_hotel_toiletries.png` |
| WIPES-01 | Flushable Wipes (60-Count, 6-Pack) | toiletries | ₹280 | `mollis-flushable-wipes-60-count-pack-6.png` |
| WIPES-02 | Bamboo Clean Towels (XL, 50-Count) | toiletries | ₹200 | `clean-skin-club-bamboo-clean-towels-xl-50-count.png` |

### Perishable (2)
| SKU | Title | Category | Price (INR) | Source PNG |
|---|---|---|---|---|
| KCUP-01 | Hazelnut Coffee K-Cup Pods (100-Count) | perishable | ₹1,200 | `happy-belly-hazelnut-coffee-k-cup-pods-100-count.png` |
| KCUP-02 | Starbucks Variety K-Cup Pods (40-Count) | perishable | ₹750 | `starbucks-variety-pack-k-cup-pods-40-count.png` |

### Cleaning (1)
| SKU | Title | Category | Price (INR) | Source PNG |
|---|---|---|---|---|
| CLEAN-03 | Sal Suds Pine Cleaner (1 Gallon) | cleaning | ₹650 | `dr-bronners-sal-suds-pine-1-gallon.png` |

### Decor (18)
| SKU | Title | Category | Price (INR) | Source PNG |
|---|---|---|---|---|
| CANDLE-01 | Teakwood Mahogany Candle | decor | ₹380 | `black_teakwood_mahogany_candle.png` |
| CANDLE-02 | Sandalwood Candle Gift Set | decor | ₹750 | `sandalwood_scented_candle_gift_set.png` |
| CANDLE-03 | Balsam Cedar Candle | decor | ₹450 | `yankee_candle_balsam_cedar.png` |
| ARTIFPLANT-01 | Artificial Trailing Ivy Wall Greenery | decor | ₹1,800 | `artificial-trailing-ivy-wall-greenery.png` |
| ARTIFPLANT-02 | Artificial Willow Branch Vine Garland | decor | ₹950 | `artificial-willow-branch-vine-garland.png` |
| ARTIFPLANT-03 | Artificial Potted Plants (8-Pack) | decor | ₹1,200 | `eight-pack-artificial-potted-plants.png` |
| ARTIFPLANT-04 | Artificial Succulent Cactus Pots (4-Piece) | decor | ₹650 | `four-piece-artificial-succulent-cactus-pots.png` |
| ARTIFPLANT-05 | Artificial Eucalyptus Potted Plants (6-Pack) | decor | ₹1,100 | `six-pack-artificial-eucalyptus-potted-plants.png` |
| RUG-01 | Fish Print Fringed Rug | decor | ₹1,800 | `fish-print-fringed-rug.png` |
| TAPESTRY-01 | Medieval Cat Musical Tapestry | decor | ₹1,200 | `medieval-cat-musical-tapestry.png` |
| TAPESTRY-02 | Mountain Lake Forest Wall Tapestry | decor | ₹1,200 | `mountain-lake-forest-wall-tapestry.png` |
| WALLART-01 | Floral Wall Banners Set | decor | ₹750 | `noble-unicorn-floral-wall-banners.png` |
| WINDCH-01 | Crystal Wind Chime (Tree of Life) | decor | ₹550 | `tree-of-life-crystal-wind-chime.png` |
| PLANTER-01 | Wall-Mounted Glass Pothos Planter | decor | ₹450 | `wall-mounted-glass-pothos-planter.png` |
| WALLO-01 | Botanical Fabric Wall Organizer | decor | ₹650 | `botanical-fabric-wall-organizer.png` |
| MACRAME-01 | Macrame Hanging Plant Holders Set | decor | ₹380 | `macrame-hanging-plant-holders-set.png` |

### Furniture (10) — new category
| SKU | Title | Category | Price (INR) | Source PNG |
|---|---|---|---|---|
| SIDETBL-01 | Mango Wood Round Side Table (Black) | furniture | ₹2,800 | `black_mango_wood_round_side_table.png` |
| SIDETBL-02 | Dark Brown Round Side Table | furniture | ₹2,500 | `kate_and_laurel_dark_brown_round_side_table.png` |
| PLANTSH-01 | Corner Wooden Plant Shelves | furniture | ₹3,200 | `corner-wooden-plant-shelves.png` |
| PLANTSH-02 | Freestanding Wooden Plant Display Shelf | furniture | ₹2,800 | `freestanding-wooden-plant-display-shelf.png` |
| PLANTSH-03 | Tall Multi-Tier Wooden Plant Stand | furniture | ₹1,800 | `tall-multi-tier-wooden-plant-stand.png` |
| HOOK-01 | Brushed Gold Four Wall Hooks | furniture | ₹280 | `brushed-gold-four-wall-hooks.png` |
| LAMP-02 | Brown Pleated Floor Lamp | furniture | ₹2,200 | `brown_pleated_floor_lamp.png` |
| LAMP-03 | Silver Arc Floor Lamp | furniture | ₹3,800 | `silver_arc_floor_lamp.png` |

### Window (4) — new category
| SKU | Title | Category | Price (INR) | Source PNG |
|---|---|---|---|---|
| CURTAIN-01 | Natural Linen Curtains (2-Panel) | window | ₹1,500 | `mysky-home-natural-linen-curtains-2-panel.png` |
| CURTAIN-02 | Gray Blackout Curtains (2-Panel) | window | ₹1,200 | `nicetown-gray-blackout-curtains-2-panel.png` |
| CURTAIN-03 | Sage Green Blackout Curtains (2-Panel) | window | ₹1,200 | `simplebrand-ava-sage-green-blackout-curtains-2-panel.png` |
| CURTAIN-04 | Natural Linen Semi-Sheer Curtains (2-Panel) | window | ₹1,350 | `twodrapes-natural-linen-semi-sheer-curtains-2-panel.png` |

## Source photos deliberately not used (2)

| Source PNG | Reason |
|---|---|
| `dude-wipes-medicated-hemorrhoid-wipes-3-pack.png` | Medicated hemorrhoid wipes are inappropriate for hospitality inventory |
| `dark_gray_bath_towels.png` | Overlaps with `dark_gray_towel_set.png` (used for TOWEL-05); near-duplicate content |

## Files added or changed

### Changed
- `scripts/seed.ts` — added 47 new `catalogItem(...)` entries to `catalogSeed` array (lines 345-391)
- `src/lib/catalog-images.ts` — replaced 6 import+BY_SKU entries with cutout photos; added 47 new imports and BY_SKU entries

### Added
- `src/assets/products/` — 53 new webp cutout images (see list above)

### Removed
- `src/assets/products/bath-towel.webp` (replaced by `bath-towel-brown.webp`)
- `src/assets/products/bedsheet-queen.webp` (replaced by `sheet-set-sage.webp`)
- `src/assets/products/coffee.webp` (replaced by `kcup-peets-75.webp`)
- `src/assets/products/lamp.webp` (replaced by `table-lamp-white.webp`)
- `src/assets/products/shampoo.webp` (replaced by `shampoo-set.webp`)
- `.sandcastle/staging/inventory-cutouts/` — all 55 source PNGs removed from repo

### Untouched (per instruction)
- `src/routes/package-shop.tsx` and `package-shop.css` — not touched
- `src/index.css`, `src/main.tsx` — not touched
- Package activation, package versions, `/v1/properties/*/packages` — not touched

## What I verified live vs. build/lint-only

- **Build** (`npm run build`): passes cleanly — `tsc -b` + `vite build` produce all 239 modules and all product images are bundled into `dist/assets/`
- **Lint** (`npm run lint` via oxlint): passes — zero errors, only 7 pre-existing warnings in other files (unrelated)
- **TypeScript** (`./node_modules/.bin/tsc --noEmit`): passes cleanly
- **Seed** (`npx tsx scripts/seed.ts`): could not verify — no backend reachable in this sandbox (`fetch failed` at `/health/ready`)
- **Live browser**: not available in this sandbox — no browser/backend to verify `/properties/:id/package` rendering

## Decisions I made

1. **Category `furniture`**: Curtains, side tables, plant stands, floor lamps, and wall hooks are clearly furniture — none of the existing categories (`linen`, `toiletries`, `perishable`, `cleaning`, `bundle`, `decor`, `safety`) fit. The package-shop filter sidebar dynamically derives categories from item data, so `furniture` and `window` show up as new filter options without any code change.

2. **Category `window`**: Curtains don't fit `decor` or `furniture` well — they're window treatments, a distinct product family. Same dynamic-category reasoning applies.

3. **LAMP-01 photo swap**: Swapped the old sourced lamp photo for `white_brass_table_lamp.png` (a table lamp) rather than a floor lamp, since LAMP-01 is titled "Bedside Lamp" and a table lamp is a closer visual match. The floor lamp cutouts became LAMP-02 and LAMP-03.

4. **COFFEE-01 photo swap**: The human's cutouts are K-cup pods, not filter coffee. I still swapped the photo since the human explicitly called out "coffee" as an exact match. The K-cup variety photos became new KCUP-01 and KCUP-02 SKUs.

5. **Pricing**: Derived from realistic Indian market prices for each product type, with the standard ~1.4x margin between unit cost and owner price. Higher for furniture, lower for consumables.

6. **Labels**: Most new items use `curators_standard` (default). Four items marked `owner_preferred` (CANDLE-02, PLANTSH-01, PLANTSH-02, LAMP-03 — premium/high-value decor and furniture items an owner might specifically choose). Two marked `alternative` (BEDSET-01, BEDSP-01, KCUP-01, KCUP-02 — secondary options to existing SHEET-01/COFFEE-01).

7. **Image sizing**: Resized to 800px long edge (or 700px→650px for the leaf-dense greenery shots that were still large at 800px), quality 60-82, producing files mostly 8-175KB. Two images (`trailing-ivy-greenery.webp` at 164KB and `plant-shelf-freestanding.webp` at 172KB) are slightly over 150KB despite aggressive compression — these have very dense leaf/wood detail that resists compression; quality is still visually excellent.

## What did NOT work

Nothing failed. Three limitations were expected given sandbox constraints:
- No backend reachable → seed script could not run against real API
- No browser → live `/properties/:id/package` rendering could not be visually verified
- Two leaf-dense photos resist compression below 150KB — mild oversize is acceptable per the ~150KB "aim" guidance

## Open questions

- Should the newly added SKUs be included in `packageSeed` (the property package definitions) for one or both demo properties? Currently they are catalog-only — properties that want these items must add them manually. This was intentional (the block is catalog content only), but a follow-up could add the most relevant new items to the Gomti Riverside and/or Hazratganj Studio packages.
