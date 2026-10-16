import type { Operation } from '@/api/types';

export type OperationMonthSection = { key: string; title: string; data: Operation[] };

const MONTH_FORMATTER = new Intl.DateTimeFormat('fr-FR', { month: 'long', year: 'numeric' });

function monthLabel(isoDate: string): string {
  const [year, month] = isoDate.split('-').map(Number);
  const label = MONTH_FORMATTER.format(new Date(Date.UTC(year, month - 1, 1)));
  return label.charAt(0).toUpperCase() + label.slice(1);
}

/** Groups already date-sorted (newest first — see the operations API's own
 * ORDER BY) operations into one section per calendar month, for a
 * SectionList's sticky month headers. Walks the list once, grouping runs of
 * consecutive same-month operations, rather than a generic group-by-key +
 * re-sort — correct only because the input is already ordered, which every
 * caller (the operations screen, paginated newest-first) guarantees. */
export function groupOperationsByMonth(operations: Operation[]): OperationMonthSection[] {
  const sections: OperationMonthSection[] = [];
  for (const op of operations) {
    const key = op.date.slice(0, 7); // "YYYY-MM"
    const last = sections[sections.length - 1];
    if (last && last.key === key) {
      last.data.push(op);
    } else {
      sections.push({ key, title: monthLabel(op.date), data: [op] });
    }
  }
  return sections;
}
