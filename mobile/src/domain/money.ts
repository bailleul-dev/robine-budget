/** Amount formatting shared by every screen. An amount is a bare number in
 * its account's currency (Account.currency, an ISO 4217 code validated by
 * the server) — always format it with that currency, never a hardcoded €. */

/** Currencies with no minor unit: 1 234 ¥, not 1 234,00 ¥. */
const ZERO_DECIMAL_CURRENCIES = new Set(['JPY', 'XOF', 'XAF']);

const SYMBOLS: Record<string, string> = { EUR: '€', USD: '$', GBP: '£', JPY: '¥' };

export function currencySymbol(currency: string): string {
  return SYMBOLS[currency] ?? currency;
}

/** "1 234,56" — the number alone, for layouts that render the currency
 * separately (e.g. in a smaller font next to a hero total). */
export function formatAmount(amount: string | number, currency: string | undefined): string {
  const n = Number(amount);
  if (Number.isNaN(n)) return String(amount);
  const digits = currency && ZERO_DECIMAL_CURRENCIES.has(currency) ? 0 : 2;
  return n
    .toLocaleString('fr-FR', { minimumFractionDigits: digits, maximumFractionDigits: digits })
    // fr-FR groups thousands with U+202F, which Manrope has no glyph for.
    .replace(/ /g, ' ');
}

/** "1 234,56 €" — amount and currency symbol, joined by a non-breaking space.
 * `currency` is undefined only while the owning account is still loading;
 * the number then shows alone rather than with a guessed €. */
export function formatMoney(amount: string | number, currency: string | undefined): string {
  const formatted = formatAmount(amount, currency);
  return currency ? `${formatted} ${currencySymbol(currency)}` : formatted;
}
