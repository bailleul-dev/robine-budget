import { isoDate } from '@/domain/dashboard';

/** How long a window the category breakdown covers, in whole months. */
export type PeriodPreset = 'month' | '3m' | '6m' | '1y' | '2y';

export const PERIOD_PRESETS: { value: PeriodPreset; label: string; months: number }[] = [
  { value: 'month', label: 'Mois', months: 1 },
  { value: '3m', label: '3 mois', months: 3 },
  { value: '6m', label: '6 mois', months: 6 },
  { value: '1y', label: '1 an', months: 12 },
  { value: '2y', label: '2 ans', months: 24 },
];

/** A preset plus how many whole windows back from the current one (0 = the
 * window ending with this month, -1 = the one right before it, …). Stepping
 * moves by the window's own length, so "6 mois" walks back six months at a
 * time and every window lines up with the previous one, no overlap. */
export type Period = { preset: PeriodPreset; offset: number };

export type PeriodRange = {
  /** First and last day, inclusive, as YYYY-MM-DD. */
  from: string;
  to: string;
  months: number;
  label: string;
  isCurrent: boolean;
};

const MONTHS = ['janvier', 'février', 'mars', 'avril', 'mai', 'juin', 'juillet', 'août', 'septembre', 'octobre', 'novembre', 'décembre'];
const SHORT_MONTHS = ['janv.', 'févr.', 'mars', 'avr.', 'mai', 'juin', 'juil.', 'août', 'sept.', 'oct.', 'nov.', 'déc.'];

function monthsOf(preset: PeriodPreset): number {
  return PERIOD_PRESETS.find((p) => p.value === preset)!.months;
}

/** Resolves a period to concrete dates, relative to `today`. Windows are
 * whole calendar months: "6 mois" now means the first of the month five
 * months ago through the end of this month. */
export function periodRange(period: Period, today: Date = new Date()): PeriodRange {
  const months = monthsOf(period.preset);
  // Months are counted as year*12+month so arithmetic crosses years freely.
  const endIndex = today.getFullYear() * 12 + today.getMonth() + period.offset * months;
  const startIndex = endIndex - months + 1;
  const start = new Date(Math.floor(startIndex / 12), startIndex % 12, 1);
  const end = new Date(Math.floor(endIndex / 12), (endIndex % 12) + 1, 0);

  let label: string;
  if (months === 1) {
    label = `${MONTHS[start.getMonth()]} ${start.getFullYear()}`;
  } else if (start.getMonth() === 0 && end.getMonth() === 11 && start.getFullYear() === end.getFullYear()) {
    label = String(start.getFullYear());
  } else if (start.getFullYear() === end.getFullYear()) {
    label = `${SHORT_MONTHS[start.getMonth()]} – ${SHORT_MONTHS[end.getMonth()]} ${end.getFullYear()}`;
  } else {
    label = `${SHORT_MONTHS[start.getMonth()]} ${start.getFullYear()} – ${SHORT_MONTHS[end.getMonth()]} ${end.getFullYear()}`;
  }

  return { from: isoDate(start), to: isoDate(end), months, label, isCurrent: period.offset === 0 };
}
