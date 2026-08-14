// Real property photography matched by the seeded service address rather
// than property id. The backend generates prop_ ids per database seed, while
// the demo addresses are the stable identity of these two properties.

import gomtiRiverside2Bhk from "../assets/properties/gomti-riverside-2bhk.webp";
import hazratganjStudio from "../assets/properties/hazratganj-studio.webp";

export type PropertyImage = {
  src: string;
  alt: string;
};

const BY_ADDRESS: Record<string, PropertyImage> = {
  "12 gomti nagar extension|226010": {
    src: gomtiRiverside2Bhk,
    alt: "Bright residential living room with a garden-facing exterior",
  },
  "4/22 hazratganj|226001": {
    src: hazratganjStudio,
    alt: "Warm, contemporary residential interior with natural light",
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
