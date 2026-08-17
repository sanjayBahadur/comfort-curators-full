// Real product photography for catalog items, matched by genuine content —
// no forced/misleading matches (a "floor lamp" cutout does not become a
// "floor cleaner" photo just because both say "floor"). Items with no honest
// match keep the existing two-letter category badge; that's an intentional,
// already-designed fallback, not a placeholder to eliminate.
//
// Two catalogs use this: package-shop's internal/catalog (keyed by SKU) and
// stay.tsx's mock quick-commerce store (keyed by its own item id). Both
// lookups live here so there is one place that knows which real product a
// photo actually shows, and the short display label used alongside it.

import handTowel from "../assets/products/hand-towel.webp";
import tea from "../assets/products/tea.webp";
import welcomeKit from "../assets/products/welcome-kit.webp";
import soap from "../assets/products/soap.webp";
import guestBathTowel from "../assets/products/guest-bath-towel.webp";
import guestBedSheet from "../assets/products/guest-bed-sheet.webp";
import floorCleaner from "../assets/products/floor-cleaner.webp";
import dishwashLiquid from "../assets/products/dishwash-liquid.webp";
import microfiberCloths from "../assets/products/microfiber-cloths.webp";
import basmatiRice from "../assets/products/basmati-rice.webp";
import toiletCleaner from "../assets/products/toilet-cleaner.webp";

import bathTowelBrown from "../assets/products/bath-towel-brown.webp";
import sheetSetSage from "../assets/products/sheet-set-sage.webp";
import shampooSet from "../assets/products/shampoo-set.webp";
import teaTazo from "../assets/products/tea-tazo.webp";
import kcupPeets75 from "../assets/products/kcup-peets-75.webp";
import tableLampWhite from "../assets/products/table-lamp-white.webp";

import towelSetGreen from "../assets/products/towel-set-green.webp";
import towelSetMulticolor from "../assets/products/towel-set-multicolor.webp";
import towelSetDarkgray from "../assets/products/towel-set-darkgray.webp";
import sheetSetBamboo from "../assets/products/sheet-set-bamboo.webp";
import comforterSetQueen from "../assets/products/comforter-set-queen.webp";
import quiltedBedspread from "../assets/products/quilted-bedspread.webp";
import toothbrushes36pk from "../assets/products/toothbrushes-36pk.webp";
import toothbrushes160pk from "../assets/products/toothbrushes-160pk.webp";
import toothbrushes20pk from "../assets/products/toothbrushes-20pk.webp";
import toothpasteMultipack from "../assets/products/toothpaste-multipack.webp";
import toiletriesTropicalOasis from "../assets/products/toiletries-tropical-oasis.webp";
import toiletriesTropicalOasis200 from "../assets/products/toiletries-tropical-oasis-200.webp";
import toiletriesWhiteTea from "../assets/products/toiletries-white-tea.webp";
import toiletriesNatureEssence from "../assets/products/toiletries-nature-essence.webp";
import wipesFlushable6pk from "../assets/products/wipes-flushable-6pk.webp";
import bambooCleanTowels from "../assets/products/bamboo-clean-towels.webp";
import kcupHazelnut100 from "../assets/products/kcup-hazelnut-100.webp";
import kcupStarbucks40 from "../assets/products/kcup-starbucks-40.webp";
import salSudsCleaner from "../assets/products/sal-suds-cleaner.webp";
import candleBalsamCedar from "../assets/products/candle-balsam-cedar.webp";
import trailingIvyGreenery from "../assets/products/trailing-ivy-greenery.webp";
import willowGarland from "../assets/products/willow-garland.webp";
import pottedPlants8pk from "../assets/products/potted-plants-8pk.webp";
import succulentPots4pk from "../assets/products/succulent-pots-4pk.webp";
import pottedEucalyptus6pk from "../assets/products/potted-eucalyptus-6pk.webp";
import fringedRugFish from "../assets/products/fringed-rug-fish.webp";
import tapestryMedievalCat from "../assets/products/tapestry-medieval-cat.webp";
import tapestryMountainLake from "../assets/products/tapestry-mountain-lake.webp";
import wallBannersFloral from "../assets/products/wall-banners-floral.webp";
import windChime from "../assets/products/wind-chime.webp";
import planterPothosWall from "../assets/products/planter-pothos-wall.webp";
import wallOrganizer from "../assets/products/wall-organizer.webp";
import macrameHolders from "../assets/products/macrame-holders.webp";
import sideTableMango from "../assets/products/side-table-mango.webp";
import sideTableDarkbrown from "../assets/products/side-table-darkbrown.webp";
import plantShelvesCorner from "../assets/products/plant-shelves-corner.webp";
import plantShelfFreestanding from "../assets/products/plant-shelf-freestanding.webp";
import plantStandTall from "../assets/products/plant-stand-tall.webp";
import wallHooksGold from "../assets/products/wall-hooks-gold.webp";
import floorLampBrown from "../assets/products/floor-lamp-brown.webp";
import floorLampSilver from "../assets/products/floor-lamp-silver.webp";
import curtainsLinen from "../assets/products/curtains-linen.webp";
import curtainsBlackoutGray from "../assets/products/curtains-blackout-gray.webp";
import curtainsBlackoutSage from "../assets/products/curtains-blackout-sage.webp";
import curtainsSemiSheer from "../assets/products/curtains-semi-sheer.webp";

export type CatalogImage = {
  src: string;
  /** Short, general classification for display alongside the real product
   *  name -- "Bed Sheet", not the photo's own brand/product title. */
  label: string;
};

// package-shop's internal/catalog items, keyed by SKU.
const BY_SKU: Record<string, CatalogImage> = {
  "LAMP-01": { src: tableLampWhite, label: "Lamp" },
  "SHEET-01": { src: sheetSetSage, label: "Bed Sheet" },
  "TOWEL-01": { src: bathTowelBrown, label: "Bath Towel" },
  "TOWEL-02": { src: handTowel, label: "Hand Towel" },
  "TEA-01": { src: teaTazo, label: "Tea" },
  "COFFEE-01": { src: kcupPeets75, label: "Coffee" },
  "KIT-01": { src: welcomeKit, label: "Welcome Kit" },
  "SHAMP-01": { src: shampooSet, label: "Shampoo" },
  "SOAP-01": { src: soap, label: "Soap" },

  "TOWEL-03": { src: towelSetGreen, label: "Towel Set" },
  "TOWEL-04": { src: towelSetMulticolor, label: "Towel Set" },
  "TOWEL-05": { src: towelSetDarkgray, label: "Towel Set" },
  "SHEET-02": { src: sheetSetBamboo, label: "Bed Sheet" },
  "BEDSET-01": { src: comforterSetQueen, label: "Bedding Set" },
  "BEDSP-01": { src: quiltedBedspread, label: "Bedspread" },
  "TOOTHBR-01": { src: toothbrushes36pk, label: "Toothbrushes" },
  "TOOTHBR-02": { src: toothbrushes160pk, label: "Toothbrushes" },
  "TOOTHBR-03": { src: toothbrushes20pk, label: "Toothbrushes" },
  "TOOTHPA-01": { src: toothpasteMultipack, label: "Toothpaste" },
  "TOILETRY-01": { src: toiletriesTropicalOasis, label: "Toiletries Set" },
  "TOILETRY-02": { src: toiletriesTropicalOasis200, label: "Toiletries Set" },
  "TOILETRY-03": { src: toiletriesWhiteTea, label: "Toiletries Set" },
  "TOILETRY-04": { src: toiletriesNatureEssence, label: "Toiletries Set" },
  "WIPES-01": { src: wipesFlushable6pk, label: "Wipes" },
  "WIPES-02": { src: bambooCleanTowels, label: "Towels" },
  "KCUP-01": { src: kcupHazelnut100, label: "Coffee Pods" },
  "KCUP-02": { src: kcupStarbucks40, label: "Coffee Pods" },
  "CLEAN-03": { src: salSudsCleaner, label: "Cleaner" },
  // CANDLE-01/02 intentionally have no photo: their source cutouts
  // (black_teakwood_mahogany_candle.png, sandalwood_scented_candle_gift_set.png)
  // both actually depict candles branded "Sensodyne Pronamel" toothpaste --
  // a mislabeled/mismatched source asset, not a real product photo. A wrong
  // photo is worse than no photo; falls back to the text-mark treatment
  // until a real matching image is sourced.
  "CANDLE-03": { src: candleBalsamCedar, label: "Candle" },
  "ARTIFPLANT-01": { src: trailingIvyGreenery, label: "Greenery" },
  "ARTIFPLANT-02": { src: willowGarland, label: "Garland" },
  "ARTIFPLANT-03": { src: pottedPlants8pk, label: "Potted Plants" },
  "ARTIFPLANT-04": { src: succulentPots4pk, label: "Succulents" },
  "ARTIFPLANT-05": { src: pottedEucalyptus6pk, label: "Potted Plants" },
  "RUG-01": { src: fringedRugFish, label: "Rug" },
  "TAPESTRY-01": { src: tapestryMedievalCat, label: "Tapestry" },
  "TAPESTRY-02": { src: tapestryMountainLake, label: "Tapestry" },
  "WALLART-01": { src: wallBannersFloral, label: "Wall Art" },
  "WINDCH-01": { src: windChime, label: "Wind Chime" },
  "PLANTER-01": { src: planterPothosWall, label: "Planter" },
  "WALLO-01": { src: wallOrganizer, label: "Wall Organizer" },
  "MACRAME-01": { src: macrameHolders, label: "Plant Holders" },
  "SIDETBL-01": { src: sideTableMango, label: "Side Table" },
  "SIDETBL-02": { src: sideTableDarkbrown, label: "Side Table" },
  "PLANTSH-01": { src: plantShelvesCorner, label: "Plant Shelf" },
  "PLANTSH-02": { src: plantShelfFreestanding, label: "Plant Shelf" },
  "PLANTSH-03": { src: plantStandTall, label: "Plant Stand" },
  "HOOK-01": { src: wallHooksGold, label: "Wall Hooks" },
  "LAMP-02": { src: floorLampBrown, label: "Floor Lamp" },
  "LAMP-03": { src: floorLampSilver, label: "Floor Lamp" },
  "CURTAIN-01": { src: curtainsLinen, label: "Curtains" },
  "CURTAIN-02": { src: curtainsBlackoutGray, label: "Curtains" },
  "CURTAIN-03": { src: curtainsBlackoutSage, label: "Curtains" },
  "CURTAIN-04": { src: curtainsSemiSheer, label: "Curtains" },
};

export function getPackageCatalogImage(sku: string): CatalogImage | undefined {
  return BY_SKU[sku];
}

// stay.tsx's mock quick-commerce store (internal/procurement/store/mock.go),
// keyed by its own item id.
const BY_STORE_ITEM_ID: Record<string, CatalogImage> = {
  mock_catalog_tea: { src: tea, label: "Tea" },
  mock_catalog_cotton_towels: { src: guestBathTowel, label: "Bath Towel" },
  mock_catalog_bed_sheet: { src: guestBedSheet, label: "Bed Sheet" },
  mock_catalog_floor_cleaner: { src: floorCleaner, label: "Floor Cleaner" },
  mock_catalog_dishwash_liquid: { src: dishwashLiquid, label: "Dishwash Liquid" },
  mock_catalog_microfiber_cloths: { src: microfiberCloths, label: "Cleaning Cloths" },
  mock_catalog_basmati_rice: { src: basmatiRice, label: "Basmati Rice" },
  mock_catalog_toilet_cleaner: { src: toiletCleaner, label: "Toilet Cleaner" },
  // mock_catalog_paper_towels intentionally has no photo: every sourced
  // candidate for "kitchen paper towels" was visually indistinguishable
  // from a toilet-paper roll -- a wrong photo is worse than no photo (see
  // the CANDLE-01/02 precedent above). Falls back to the category badge
  // until a real, honest match is sourced.
};

export function getStoreCatalogImage(itemId: string): CatalogImage | undefined {
  return BY_STORE_ITEM_ID[itemId];
}
