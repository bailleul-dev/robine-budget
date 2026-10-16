import type { Account, Category, Operation } from '@/api/types';
import { categoryDisplayName, rootCategoryOf } from '@/domain/category';

export type CategorySlice = { categoryId: string; name: string; amount: number };

export type CurrencyBreakdown = { currency: string; slices: CategorySlice[]; total: number };

/** Distinct, soft palette assigned by rank (biggest slice first) rather
 * than tied to any category — simplest way to get consistent, non-clashing
 * colors without a per-category color field in the backend. Repeats past
 * 10 slices, which in practice never happens (there are 9 standard
 * top-level categories). Lagune hues — lagoon, lilac, sky, peach, coral,
 * mint — matching the app's identity (see constants/theme.ts) rather than
 * generic saturated chart colors. */
const SLICE_COLORS = [
  '#23A898',
  '#9D8FF0',
  '#4AA9D6',
  '#F59A6C',
  '#E0626F',
  '#7ED3B2',
  '#F7C969',
  '#6C7FD8',
  '#E58FC0',
  '#8AA3B0',
];

/** Aggregates a month's worth of operations into a spending-by-category
 * breakdown, one currency group per account currency (amounts in different
 * currencies can't share a pie) and one slice per top-level (standard)
 * category — a subcategory's spending rolls up into its root via
 * rootCategoryOf, matching how the web app's category picker groups
 * things. Only expenses (negative amounts) count; income and transfers
 * don't belong on a "where did my money go" chart. A split operation
 * contributes each split's own amount instead of the operation's total.
 * Operations with no category (and no splits) are grouped under "Non
 * catégorisé" so the total still reconciles. */
export function buildCategoryBreakdown(
  categories: Category[],
  accountsWithOps: { account: Account; operations: Operation[] }[],
): CurrencyBreakdown[] {
  const byId = new Map(categories.map((c) => [c.id, c]));
  const byCurrency = new Map<string, Map<string, number>>();
  const namesByKey = new Map<string, string>();

  const add = (currency: string, categoryId: string | null, amount: number) => {
    if (amount >= 0) return;
    const spent = -amount;
    let key = 'uncategorized';
    let name = 'Non catégorisé';
    if (categoryId) {
      const category = byId.get(categoryId);
      if (category) {
        const root = rootCategoryOf(category, categories);
        key = root.id;
        name = categoryDisplayName(root);
      }
    }
    namesByKey.set(key, name);
    const totals = byCurrency.get(currency) ?? new Map<string, number>();
    totals.set(key, (totals.get(key) ?? 0) + spent);
    byCurrency.set(currency, totals);
  };

  for (const { account, operations } of accountsWithOps) {
    for (const op of operations) {
      if (op.splits.length > 0) {
        for (const split of op.splits) add(account.currency, split.category_id, Number(split.amount));
      } else {
        add(account.currency, op.category_id, Number(op.amount));
      }
    }
  }

  return [...byCurrency.entries()].map(([currency, totals]) => {
    const slices = [...totals.entries()]
      .map(([categoryId, amount]) => ({ categoryId, name: namesByKey.get(categoryId) ?? '', amount }))
      .sort((a, b) => b.amount - a.amount);
    return { currency, slices, total: slices.reduce((sum, s) => sum + s.amount, 0) };
  });
}

export function colorForSliceIndex(index: number): string {
  return SLICE_COLORS[index % SLICE_COLORS.length];
}
