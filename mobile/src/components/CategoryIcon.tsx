import type { Category } from '@/api/types';
import { rootCategoryOf } from '@/domain/category';
import { NavIcon } from '@/components/nav-icon';
import type { AndroidSymbol, SFSymbol } from 'expo-symbols';

type IconName = { ios: SFSymbol; android: AndroidSymbol; web: AndroidSymbol };

/** Icon per standard top-level category name (seeded in backend migrations
 * — see backend/migrations/000004_add_categories.up.sql), mirroring web's
 * CategoryIcon.tsx (hand-inked SVGs there; expo-symbols' cross-platform
 * icon set here, same as the nav drawer). A category not in this map (any
 * subcategory, or a name that doesn't match) falls back to a generic dot. */
const ICON_BY_TOP_LEVEL_NAME: Record<string, IconName> = {
  Income: { ios: 'banknote.fill', android: 'payments', web: 'payments' },
  Housing: { ios: 'house.fill', android: 'home', web: 'home' },
  Transport: { ios: 'car.fill', android: 'directions_car', web: 'directions_car' },
  'Food & Drink': { ios: 'fork.knife', android: 'restaurant', web: 'restaurant' },
  Shopping: { ios: 'bag.fill', android: 'shopping_bag', web: 'shopping_bag' },
  'Bills & Subscriptions': { ios: 'doc.text.fill', android: 'receipt_long', web: 'receipt_long' },
  Health: { ios: 'cross.case.fill', android: 'local_hospital', web: 'local_hospital' },
  Entertainment: { ios: 'film.fill', android: 'movie', web: 'movie' },
  'Savings & Investments': { ios: 'chart.line.uptrend.xyaxis', android: 'savings', web: 'savings' },
  'Family & Children': {
    ios: 'figure.2.and.child.holdinghands',
    android: 'family_restroom',
    web: 'family_restroom',
  },
  Taxes: { ios: 'building.columns.fill', android: 'account_balance', web: 'account_balance' },
  'Gifts & Donations': { ios: 'gift.fill', android: 'card_giftcard', web: 'card_giftcard' },
  Pets: { ios: 'pawprint.fill', android: 'pets', web: 'pets' },
  Other: { ios: 'ellipsis.circle.fill', android: 'more_horiz', web: 'more_horiz' },
};

/** Resolves `category`'s icon via its top-level ancestor in `allCategories`
 * — a subcategory or a user's own custom one shows the same icon as the
 * standard branch it lives under. */
export function CategoryIcon({
  category,
  allCategories,
  size = 16,
  color,
}: {
  category: Category;
  allCategories: Category[];
  size?: number;
  color: string;
}) {
  const root = rootCategoryOf(category, allCategories);
  const icon = ICON_BY_TOP_LEVEL_NAME[root.name];
  if (!icon) return null;
  return <NavIcon name={icon} color={color} size={size} />;
}
