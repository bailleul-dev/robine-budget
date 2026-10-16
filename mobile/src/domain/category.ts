import type { Category } from '@/api/types';

/** Same map as frontend/src/domain/category.ts's STANDARD_CATEGORY_KEYS, but
 * resolved straight to the French label — the mobile app has no i18n layer
 * yet (every screen is hardcoded French), so there's no key indirection to
 * keep here. A user's own custom category isn't in this map and is shown
 * as typed. */
const STANDARD_CATEGORY_NAMES_FR: Record<string, string> = {
  Income: 'Revenus',
  Salary: 'Salaire',
  'Other income': 'Autres revenus',
  Housing: 'Logement',
  'Rent / mortgage': 'Loyer / crédit',
  Utilities: 'Charges',
  Water: 'Eau',
  Electricity: 'Électricité',
  Transport: 'Transport',
  'Public transit': 'Transports en commun',
  'Fuel & parking': 'Carburant & stationnement',
  'Food & Drink': 'Alimentation',
  Groceries: 'Courses',
  Restaurants: 'Restaurants',
  Shopping: 'Achats',
  Clothing: 'Vêtements',
  Electronics: 'Électronique',
  'Bills & Subscriptions': 'Factures & abonnements',
  Insurance: 'Assurance',
  Subscriptions: 'Abonnements',
  Internet: 'Internet',
  Phone: 'Téléphone',
  'Bank fees': 'Frais bancaires',
  Health: 'Santé',
  'Doctor & pharmacy': 'Médecin & pharmacie',
  Fitness: 'Sport',
  Optical: 'Optique',
  Dental: 'Dentaire',
  Therapy: 'Thérapie & psy',
  Entertainment: 'Loisirs',
  Leisure: 'Détente',
  Travel: 'Voyages',
  Culture: 'Culture',
  'Sports & activities': 'Sport & activités',
  Streaming: 'Streaming',
  'Musical instruments': 'Instruments de musique',
  'Creative hobbies': 'Loisirs créatifs',
  'Savings & Investments': 'Épargne & Investissements',
  'Savings transfer': 'Virement épargne',
  Investments: 'Investissements',
  Crypto: 'Crypto',
  'Family & Children': 'Famille & Enfants',
  Childcare: 'Crèche & garde',
  'School fees': 'Frais de scolarité',
  'Child support': 'Pension alimentaire',
  "Children's activities": 'Activités enfants',
  Taxes: 'Impôts & Taxes',
  'Income tax': 'Impôt sur le revenu',
  'Property tax': 'Taxe foncière / habitation',
  Fines: 'Amendes',
  'Gifts & Donations': 'Cadeaux & Dons',
  Gifts: 'Cadeaux',
  'Charity & donations': 'Dons & charité',
  Pets: 'Animaux',
  Veterinary: 'Vétérinaire',
  'Pet food & supplies': 'Nourriture & accessoires',
  'Car purchase / loan': 'Achat / crédit véhicule',
  'Car maintenance & repair': 'Entretien & réparation',
  'Taxi & ride-hailing': 'Taxi & VTC',
  'Coffee & bakery': 'Café & boulangerie',
  Alcohol: 'Alcool',
  Tobacco: 'Tabac',
  'Home & furniture': 'Maison & ameublement',
  'Beauty & care': 'Beauté & soins',
  Other: 'Autre',
};

export function categoryDisplayName(category: Category): string {
  return STANDARD_CATEGORY_NAMES_FR[category.name] ?? category.name;
}

export type CategoryTreeEntry = { category: Category; depth: number };

/** Flattens the parent_id tree into a root-first, alphabetically-sorted
 * (per sibling level) list with a depth per entry, for the category picker.
 * Ported from frontend/src/domain/category.ts's flattenCategoryTree. */
export function flattenCategoryTree(categories: Category[]): CategoryTreeEntry[] {
  const byParent = new Map<string | null, Category[]>();
  for (const c of categories) {
    const siblings = byParent.get(c.parent_id) ?? [];
    siblings.push(c);
    byParent.set(c.parent_id, siblings);
  }
  for (const siblings of byParent.values()) {
    siblings.sort((a, b) => categoryDisplayName(a).localeCompare(categoryDisplayName(b)));
  }

  const result: CategoryTreeEntry[] = [];
  const visit = (parentId: string | null, depth: number) => {
    for (const c of byParent.get(parentId) ?? []) {
      result.push({ category: c, depth });
      visit(c.id, depth + 1);
    }
  };
  visit(null, 0);
  return result;
}

/** Walks a category's parent_id chain up to its top-level (standard)
 * ancestor — e.g. "Farmers market" under "Groceries" under "Food & Drink"
 * resolves to "Food & Drink". Ported from frontend/src/domain/category.ts,
 * used here to group the spending pie chart by top-level category instead
 * of by every leaf. */
export function rootCategoryOf(category: Category, all: Category[]): Category {
  const byId = new Map(all.map((c) => [c.id, c]));
  let current = category;
  while (current.parent_id !== null) {
    const parent = byId.get(current.parent_id);
    if (!parent) break;
    current = parent;
  }
  return current;
}
