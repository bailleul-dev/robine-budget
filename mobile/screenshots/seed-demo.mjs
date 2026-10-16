#!/usr/bin/env node
// Fills a *fresh* Budget server with a believable demo user for App Store
// screenshots — see screenshots/run.sh. Everything goes through the public
// API, exactly like the app itself: accounts, recurring budgets and
// envelopes, three months of operations imported as a Crédit Mutuel CSV,
// categories, and today's balances.
//
//   API_URL=http://127.0.0.1:8010/api node screenshots/seed-demo.mjs
//
// Dates are relative to today and amounts come from a seeded generator, so
// every run tells the same story. Meant for a throwaway database: running
// it twice against the same server fails on the already-registered email.

const API_URL = (process.env.API_URL ?? 'http://127.0.0.1:8010/api').replace(/\/+$/, '');
export const DEMO_EMAIL = process.env.DEMO_EMAIL ?? 'camille@lagune.app';
export const DEMO_PASSWORD = process.env.DEMO_PASSWORD ?? 'lagune-demo-2026';

let token = null;

async function api(method, path, body) {
  const headers = { Accept: 'application/json' };
  if (token) headers.Authorization = `Bearer ${token}`;
  let payload = body;
  if (body !== undefined && !(body instanceof FormData)) {
    headers['Content-Type'] = 'application/json';
    payload = JSON.stringify(body);
  }
  const res = await fetch(`${API_URL}${path}`, { method, headers, body: payload });
  const text = await res.text();
  if (!res.ok) throw new Error(`${method} ${path} → ${res.status} ${text}`);
  return text ? JSON.parse(text) : null;
}

// --- Dates & a deterministic generator ----------------------------------

const today = new Date();
today.setHours(12, 0, 0, 0);
const iso = (d) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
const frDate = (d) => `${String(d.getDate()).padStart(2, '0')}/${String(d.getMonth() + 1).padStart(2, '0')}/${d.getFullYear()}`;
const daysAgo = (n) => new Date(today.getFullYear(), today.getMonth(), today.getDate() - n, 12);
/** Day `day` of the month `offset` months from now (clamped to its length). */
const monthDay = (offset, day) => {
  const last = new Date(today.getFullYear(), today.getMonth() + offset + 1, 0).getDate();
  return new Date(today.getFullYear(), today.getMonth() + offset, Math.min(day, last), 12);
};

let seed = 20260917;
const rand = () => {
  seed = (seed * 1103515245 + 12345) % 2147483648;
  return seed / 2147483648;
};
const between = (min, max) => Math.round((min + rand() * (max - min)) * 100) / 100;
const pick = (list) => list[Math.floor(rand() * list.length)];

// --- The story -----------------------------------------------------------

const HISTORY_DAYS = 92;

/** Operations on the current account, oldest first: [date, label, amount, categoryName]. */
function currentAccountOperations() {
  const ops = [];
  const add = (date, label, amount, category) => {
    if (date <= today && date >= daysAgo(HISTORY_DAYS)) ops.push([date, label, amount, category]);
  };

  for (let m = -3; m <= 0; m++) {
    add(monthDay(m, 28), 'VIR SALAIRE ATELIER NOVA', 2650, 'Salary');
    add(monthDay(m, 5), 'PRLV LOYER FONCIA', -890, 'Rent / mortgage');
    add(monthDay(m, 2), 'NAVIGO MOIS', -86.4, 'Public transit');
    add(monthDay(m, 12), 'PRLV EDF', -62, 'Electricity');
    add(monthDay(m, 8), 'PRLV MAIF ASSURANCE', -34.2, 'Insurance');
    add(monthDay(m, 15), 'PRLV SPOTIFY', -11.99, 'Streaming');
    add(monthDay(m, 18), 'PRLV FREE MOBILE', -15.99, 'Phone');
    add(monthDay(m, 29), 'VIR LIVRET A', -300, 'Savings transfer');
  }

  for (let d = HISTORY_DAYS; d >= 0; d--) {
    const date = daysAgo(d);
    const weekday = date.getDay();
    if (weekday === 6 || (weekday === 3 && rand() < 0.6)) {
      add(date, pick(['CB MONOPRIX', 'CB BIOCOOP', 'CB CARREFOUR MARKET']), -between(24, 92), 'Groceries');
    }
    if ((weekday === 5 || weekday === 6) && rand() < 0.45) {
      add(date, pick(['CB LE PETIT ZINC', 'CB SUSHI SHOP', 'CB CAFE POMPON', 'CB PIZZERIA BELLA']), -between(14, 48), 'Restaurants');
    }
    if (weekday === 0 && rand() < 0.3) add(date, pick(['CB CINEMA UGC', 'CB FNAC']), -between(11, 34), 'Culture');
    if (rand() < 0.04) add(date, 'CB PHARMACIE DU MARCHE', -between(6, 28), 'Doctor & pharmacy');
    if (rand() < 0.03) add(date, 'CB DECATHLON', -between(19, 64), 'Sports & activities');
  }

  return ops.sort((a, b) => a[0] - b[0]);
}

function savingsOperations() {
  const ops = [];
  for (let m = -3; m <= 0; m++) {
    const date = monthDay(m, 29);
    if (date <= today && date >= daysAgo(HISTORY_DAYS)) ops.push([date, 'VIR DEPUIS COMPTE COURANT', 300, 'Savings transfer']);
  }
  return ops;
}

/** Crédit Mutuel's export (the built-in global bank profile): ";" delimited,
 * Latin-1, dd/mm/yyyy, French decimals, signed Débit/Crédit columns, and
 * the running balance after each line — which the account screen shows as
 * its current balance, so it has to end on `finalBalance`. */
function creditMutuelCsv(ops, finalBalance) {
  const money = (n) => n.toFixed(2).replace('.', ',');
  const lines = ['Date;Date de valeur;Débit;Crédit;Libellé;Solde'];
  let balance = finalBalance - ops.reduce((sum, [, , amount]) => sum + amount, 0);
  for (const [date, label, amount] of ops) {
    balance += amount;
    lines.push([frDate(date), frDate(date), amount < 0 ? money(amount) : '', amount > 0 ? money(amount) : '', label, money(balance)].join(';'));
  }
  return Buffer.from(lines.join('\r\n') + '\r\n', 'latin1');
}

// --- Seeding -------------------------------------------------------------

async function importOperations(account, profile, ops, finalBalance, categoryIds) {
  const form = new FormData();
  form.append('bank_profile_id', profile.id);
  form.append('file', new Blob([creditMutuelCsv(ops, finalBalance)], { type: 'text/csv' }), 'releve.csv');
  const batch = await api('POST', `/accounts/${account.id}/imports`, form);
  if (batch.errors.length) throw new Error(`import into ${account.name}: ${batch.errors.join(', ')}`);

  const byLabel = new Map(ops.map(([, label, , category]) => [label, category]));
  let cursor = null;
  do {
    const page = await api('GET', `/accounts/${account.id}/operations?limit=500${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`);
    for (const op of page.operations) {
      const categoryId = categoryIds.get(byLabel.get(op.label));
      if (categoryId && op.category_id !== categoryId) {
        await api('PUT', `/operations/${op.id}/category`, { category_id: categoryId });
      }
    }
    cursor = page.next_cursor;
  } while (cursor);
  return batch.imported_count;
}

async function plannedItem(account, item) {
  const created = await api('POST', '/planned-items', {
    account_id: account.id,
    kind: item.kind ?? 'charge',
    match_mode: item.categoryId ? 'category' : 'label',
    label: item.label ?? null,
    category_id: item.categoryId ?? null,
    amount: item.amount.toFixed(2),
    // From this month: earlier months would only add past occurrences
    // left unreconciled ("en attente de rapprochement").
    start_date: iso(monthDay(0, 1)),
    recurrence_type: 'perpetual',
    day_of_month: item.day ?? 1,
    active: true,
  });
  await api('POST', `/planned-items/${created.id}/generate-occurrences?months_ahead=12`);
}

async function main() {
  const auth = await api('POST', '/auth/register', {
    email: DEMO_EMAIL,
    password: DEMO_PASSWORD,
    display_name: 'Camille',
  });
  token = auth.token;

  const categories = await api('GET', '/categories');
  const categoryIds = new Map(categories.map((c) => [c.name, c.id]));
  const profiles = await api('GET', '/bank-profiles');
  const profile = profiles.find((p) => p.global && p.name === 'Crédit Mutuel');
  if (!profile) throw new Error('the global "Crédit Mutuel" bank profile is missing');

  const current = await api('POST', '/accounts', { name: 'Compte courant', currency: 'EUR' });
  const savings = await api('POST', '/accounts', { name: 'Livret A', currency: 'EUR' });
  const trip = await api('POST', '/accounts', { name: 'Vacances à Lisbonne', currency: 'EUR' });

  // Budgets first, so importing the operations reconciles them right away.
  await plannedItem(current, { kind: 'income', label: 'VIR SALAIRE ATELIER NOVA', amount: 2650, day: 28 });
  await plannedItem(current, { label: 'PRLV LOYER FONCIA', amount: 890, day: 5 });
  await plannedItem(current, { label: 'PRLV EDF', amount: 62, day: 12 });
  await plannedItem(current, { label: 'NAVIGO MOIS', amount: 86.4, day: 2 });
  await plannedItem(current, { label: 'PRLV SPOTIFY', amount: 11.99, day: 15 });
  await plannedItem(current, { categoryId: categoryIds.get('Groceries'), amount: 420 });
  await plannedItem(current, { categoryId: categoryIds.get('Restaurants'), amount: 160 });
  await plannedItem(current, { categoryId: categoryIds.get('Culture'), amount: 60 });

  const tripOps = [
    [daysAgo(40), 'VIR EPARGNE VOYAGE', 450, 'Savings transfer'],
    [daysAgo(10), 'VIR EPARGNE VOYAGE', 450, 'Savings transfer'],
  ];
  const accounts = [
    [current, currentAccountOperations(), 2184.37],
    [savings, savingsOperations(), 8450],
    [trip, tripOps, 1240],
  ];
  let imported = 0;
  for (const [account, ops, balance] of accounts) {
    imported += await importOperations(account, profile, ops, balance, categoryIds);
    await api('POST', `/accounts/${account.id}/balance-checkpoints/initial`, {
      as_of_date: iso(today),
      balance: balance.toFixed(2),
    });
  }

  console.log(`✔ Demo user ${DEMO_EMAIL} ready: 3 accounts, ${imported} operations.`);
}

main().catch((err) => {
  console.error(`✖ ${err.message}`);
  process.exit(1);
});
