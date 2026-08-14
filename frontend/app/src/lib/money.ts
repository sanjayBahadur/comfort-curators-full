const formatterCache = new Map<string, Intl.NumberFormat>();

function getFormatter(currency: string): Intl.NumberFormat {
  const normalizedCurrency = currency.toUpperCase();
  const cached = formatterCache.get(normalizedCurrency);
  if (cached) return cached;

  const formatter = new Intl.NumberFormat("en-IN", {
    style: "currency",
    currency: normalizedCurrency,
    currencyDisplay: "narrowSymbol",
  });
  formatterCache.set(normalizedCurrency, formatter);
  return formatter;
}

/** Formats integer minor units without converting the fractional amount to a float. */
export function formatMoney(minorUnits: number | bigint, currency: string): string {
  if (typeof minorUnits === "number" && !Number.isSafeInteger(minorUnits)) {
    throw new RangeError("Money must be provided as safe integer minor units");
  }

  const amount = BigInt(minorUnits);
  const formatter = getFormatter(currency);
  const fractionDigits = formatter.resolvedOptions().maximumFractionDigits ?? 2;
  const divisor = 10n ** BigInt(fractionDigits);
  const absolute = amount < 0n ? -amount : amount;
  const whole = Number(absolute / divisor);
  const signedWhole = amount < 0n ? -whole : whole;
  const fraction = (absolute % divisor).toString().padStart(fractionDigits, "0");

  return formatter
    .formatToParts(signedWhole)
    .map((part) => (part.type === "fraction" ? fraction : part.value))
    .join("");
}
