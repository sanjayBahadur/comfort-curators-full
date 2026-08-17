// Real property photography matched by the seeded service address rather
// than property id. The backend generates prop_ ids per database seed, while
// the demo addresses are the stable identity of these two properties.

import gomtiRiverside2Bhk from "../assets/properties/gomti-riverside-2bhk.webp";
import hazratganjStudio from "../assets/properties/hazratganj-studio.webp";
import indiraNagarVilla from "../assets/properties/indira-nagar-villa.webp";
import aliganjResidency from "../assets/properties/aliganj-residency.webp";
import mahanagarSuite from "../assets/properties/mahanagar-suite.webp";

export type PropertyImage = {
  src: string;
  alt: string;
};

const BY_ADDRESS: Record<string, PropertyImage> = {
  // Postal codes here must match scripts/seed.ts's propertySeed exactly --
  // this table matches by (line1, postal_code), not by property id (ids
  // regenerate on every reseed). Confirmed live: after moving the seed
  // data's postal codes from their old Lucknow (226xxx) range to real
  // Noida-range PINs, this table still had the old codes, so every one of
  // these lookups silently stopped matching -- not a data loss, just a key
  // mismatch, but it read as every property's curated photo having
  // vanished.
  "12 gomti nagar extension|201301": {
    src: gomtiRiverside2Bhk,
    alt: "Bright residential living room with a garden-facing exterior",
  },
  "4/22 hazratganj|201304": {
    src: hazratganjStudio,
    alt: "Warm, contemporary residential interior with natural light",
  },
  "45 indira nagar sector c|201305": {
    src: indiraNagarVilla,
    alt: "Warmly lit independent villa exterior with a landscaped garden",
  },
  "18/7 aliganj sector e|201307": {
    src: aliganjResidency,
    alt: "Open-plan living room and kitchen with warm modern finishes",
  },
  "9 mahanagar colony|201303": {
    src: mahanagarSuite,
    alt: "Serene bedroom suite with a garden-facing window",
  },
};

type PropertyAddress = {
  line1: string;
  postal_code: string;
};

export function getPropertyImage(address: PropertyAddress): PropertyImage | undefined {
  const key = `${address.line1.trim().toLocaleLowerCase("en-IN")}|${address.postal_code}`;
  return BY_ADDRESS[key];
}
