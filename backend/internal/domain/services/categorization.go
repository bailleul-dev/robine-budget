// Use-case: suggest a category for an operation from its label alone, with
// no per-user training data to learn from up front (a fresh account starts
// with zero categorized operations — there's nothing to fit a statistical
// model on). Instead of pretending otherwise, this uses three deterministic,
// explainable mechanisms, tried in order:
//
//  1. This account owner's own history: has *this* user ever categorized an
//     operation with this exact normalized label before, on any of their
//     accounts? If so, reuse their own most common choice for it — a user's
//     own corrections always win, over anyone else's and over the
//     dictionary below.
//  2. Cross-user learning: has *any* user in the system ever categorized an
//     operation with this exact normalized label? If so, reuse whichever
//     category is most common for it system-wide (see
//     ports.OperationRepository.CategoryCountsForLabels — one aggregate SQL
//     query, no separate "rules" table to maintain). This is what makes the
//     whole thing self-improving in production with no redeploy: every
//     category a user picks (by hand, or confirmed via mechanism 1/2/3)
//     becomes tomorrow's training data for every account, including ones
//     this dictionary could never know about (a local café, a landlord's
//     transfer label).
//  3. merchantDictionary: a hand-curated list of well-known French/global
//     merchants, utilities, insurers, banks and services mapped to a
//     standard category, matched as whole words/phrases against the
//     operation's label (see normalizeForMatching) — the seed for a case
//     where no history exists yet anywhere in the system. Only reached when
//     nobody, anywhere, has categorized this exact label before.
//
// Deliberately no statistical/ML layer (TF-IDF, Naive Bayes, embeddings) on
// top of this: mechanism 2 already turns the system's own accumulating
// corrections into "training data" without needing to fit anything, and an
// exact-label match is inherently higher precision than a learned model
// would be for the labels it actually covers.
package services

import (
	"context"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
)

// merchantRule maps one whole word or phrase (already lowercase, matched at
// word boundaries — see normalizeForMatching) to a standard category name
// (must match a seeded name exactly, see migrations 000004/000011/000015).
type merchantRule struct {
	keyword  string
	category string
}

// merchantDictionary: kept deliberately conservative — a keyword only goes
// in here when it reliably means one category (no "amazon" for a specific
// subcategory, since it sells everything; no "paypal", since it's a payment
// rail with no category of its own). Ambiguous merchants get the top-level
// catch-all category (e.g. "Shopping") instead of a wrong specific guess,
// or are left out entirely rather than risk a bad classification — a miss
// stays "uncategorized" and gets a human's attention; a wrong guess doesn't
// look wrong and never does.
var merchantDictionary = []merchantRule{
	// --- Food & Drink ---------------------------------------------------
	{"carrefour", "Groceries"},
	{"leclerc", "Groceries"},
	{"e leclerc", "Groceries"},
	{"intermarche", "Groceries"},
	{"super u", "Groceries"},
	{"systeme u", "Groceries"},
	{"auchan", "Groceries"},
	{"casino", "Groceries"},
	{"monoprix", "Groceries"},
	{"franprix", "Groceries"},
	{"lidl", "Groceries"},
	{"aldi", "Groceries"},
	{"netto", "Groceries"},
	{"cora", "Groceries"},
	{"biocoop", "Groceries"},
	{"grand frais", "Groceries"},
	{"picard", "Groceries"},
	{"vival", "Groceries"},
	{"spar", "Groceries"},
	{"proxi", "Groceries"},

	{"restaurant", "Restaurants"},
	{"brasserie", "Restaurants"},
	{"pizzeria", "Restaurants"},
	{"auberge", "Restaurants"},
	{"taverne", "Restaurants"},
	{"mcdonald", "Restaurants"},
	{"quick", "Restaurants"},
	{"burger king", "Restaurants"},
	{"kfc", "Restaurants"},
	{"subway", "Restaurants"},
	{"five guys", "Restaurants"},
	{"o tacos", "Restaurants"},
	{"sushi", "Restaurants"},
	{"deliveroo", "Restaurants"},
	{"uber eats", "Restaurants"},
	{"just eat", "Restaurants"},

	{"starbucks", "Coffee & bakery"},
	{"boulangerie", "Coffee & bakery"},
	{"patisserie", "Coffee & bakery"},
	{"columbus cafe", "Coffee & bakery"},
	{"cafe", "Coffee & bakery"},

	{"tabac", "Tobacco"},
	{"nicoletabac", "Tobacco"},
	{"cave a vin", "Alcohol"},
	{"cavavin", "Alcohol"},
	{"v and b", "Alcohol"},

	// --- Transport --------------------------------------------------------
	{"sncf", "Public transit"},
	{"ratp", "Public transit"},
	{"navigo", "Public transit"},
	{"tram", "Public transit"},
	{"transpole", "Public transit"},
	{"ilevia", "Public transit"},
	{"keolis", "Public transit"},
	{"metro", "Public transit"},

	{"totalenergies", "Fuel & parking"},
	{"total energies", "Fuel & parking"},
	{"station essence", "Fuel & parking"},
	{"esso", "Fuel & parking"},
	{"shell", "Fuel & parking"},
	{"avia", "Fuel & parking"},
	{"indigo park", "Fuel & parking"},
	{"q park", "Fuel & parking"},
	{"vinci park", "Fuel & parking"},
	{"effia", "Fuel & parking"},

	{"uber", "Taxi & ride-hailing"},
	{"bolt", "Taxi & ride-hailing"},
	{"heetch", "Taxi & ride-hailing"},
	{"taxi", "Taxi & ride-hailing"},
	{"freenow", "Taxi & ride-hailing"},

	{"norauto", "Car maintenance & repair"},
	{"feu vert", "Car maintenance & repair"},
	{"speedy", "Car maintenance & repair"},
	{"midas", "Car maintenance & repair"},
	{"euromaster", "Car maintenance & repair"},
	{"garage", "Car maintenance & repair"},
	{"carglass", "Car maintenance & repair"},

	// --- Housing ------------------------------------------------------
	{"ech pret", "Rent / mortgage"},
	{"echeance pret", "Rent / mortgage"},
	{"pret immobilier", "Rent / mortgage"},
	{"credit immobilier", "Rent / mortgage"},
	{"loyer", "Rent / mortgage"},

	{"engie", "Utilities"},
	{"edf", "Electricity"},
	{"enercoop", "Electricity"},
	{"total spring", "Electricity"},
	{"eni", "Electricity"},
	{"direct energie", "Electricity"},
	{"ilek", "Electricity"},
	{"mint energie", "Electricity"},

	{"veolia", "Water"},
	{"suez", "Water"},
	{"noreade", "Water"},
	{"regie noreade", "Water"},
	{"saur", "Water"},

	{"syndic", "Utilities"},
	{"copropriete", "Utilities"},

	// --- Bills & Subscriptions --------------------------------------------
	{"orange", "Phone"},
	{"sfr", "Phone"},
	{"bouygues telecom", "Phone"},
	{"sosh", "Phone"},
	{"free mobile", "Phone"},
	{"red by sfr", "Phone"},
	{"lebara", "Phone"},
	{"lycamobile", "Phone"},

	{"free", "Internet"},
	{"numericable", "Internet"},

	{"maif", "Insurance"},
	{"macif", "Insurance"},
	{"maaf", "Insurance"},
	{"allianz", "Insurance"},
	{"axa", "Insurance"},
	{"generali", "Insurance"},
	{"matmut", "Insurance"},
	{"gmf", "Insurance"},
	{"mma", "Insurance"},
	{"groupama", "Insurance"},
	{"suravenir", "Insurance"},
	{"alan", "Insurance"},
	{"april", "Insurance"},
	{"harmonie mutuelle", "Insurance"},
	{"mgen", "Insurance"},
	{"malakoff humanis", "Insurance"},
	{"silaexpert", "Insurance"},
	{"gan assurances", "Insurance"},
	{"assurance habitation", "Insurance"},
	{"assurance auto", "Insurance"},

	{"frais de tenue de compte", "Bank fees"},
	{"comm intervention", "Bank fees"},
	{"commission intervention", "Bank fees"},
	{"cotis eurocompte", "Bank fees"},
	{"cotis euc confort", "Bank fees"},
	{"cotisation carte", "Bank fees"},
	{"frais prlv imp", "Bank fees"},
	{"agios", "Bank fees"},
	{"frais bancaires", "Bank fees"},

	{"netflix", "Streaming"},
	{"spotify", "Streaming"},
	{"deezer", "Streaming"},
	{"disney", "Streaming"},
	{"canal", "Streaming"},
	{"ocs", "Streaming"},
	{"apple tv", "Streaming"},
	{"youtube premium", "Streaming"},
	{"amazon prime video", "Streaming"},

	// --- Health -------------------------------------------------------
	{"pharmacie", "Doctor & pharmacy"},
	{"docteur", "Doctor & pharmacy"},
	{"cabinet medical", "Doctor & pharmacy"},
	{"radiologie", "Doctor & pharmacy"},
	{"laboratoire", "Doctor & pharmacy"},
	{"medecin", "Doctor & pharmacy"},

	{"optic 2000", "Optical"},
	{"alain afflelou", "Optical"},
	{"krys", "Optical"},
	{"atol", "Optical"},
	{"optical center", "Optical"},
	{"generale d optique", "Optical"},

	{"dentiste", "Dental"},
	{"cabinet dentaire", "Dental"},
	{"chirurgien dentiste", "Dental"},

	{"basic fit", "Fitness"},
	{"fitness park", "Fitness"},
	{"keep cool", "Fitness"},
	{"vitagym", "Fitness"},
	{"piscine", "Fitness"},

	{"psychologue", "Therapy"},
	{"psychiatre", "Therapy"},
	{"therapeute", "Therapy"},

	// --- Entertainment ------------------------------------------------
	{"cinema", "Culture"},
	{"gaumont", "Culture"},
	{"pathe", "Culture"},
	{"ugc", "Culture"},
	{"theatre", "Culture"},
	{"musee", "Culture"},
	{"ticketmaster", "Culture"},
	{"fnac spectacles", "Culture"},

	{"air france", "Travel"},
	{"easyjet", "Travel"},
	{"ryanair", "Travel"},
	{"booking com", "Travel"},
	{"airbnb", "Travel"},
	{"hotel", "Travel"},
	{"sncf connect", "Travel"},

	{"thomann", "Musical instruments"},
	{"star s music", "Musical instruments"},
	{"woodbrass", "Musical instruments"},

	{"cultura", "Creative hobbies"},

	// --- Shopping -------------------------------------------------------
	{"zara", "Clothing"},
	{"h m", "Clothing"},
	{"uniqlo", "Clothing"},
	{"kiabi", "Clothing"},
	{"celio", "Clothing"},
	{"jules", "Clothing"},
	{"promod", "Clothing"},
	{"decathlon", "Clothing"},

	{"darty", "Electronics"},
	{"boulanger", "Electronics"},
	{"fnac", "Electronics"},
	{"apple store", "Electronics"},

	{"ikea", "Home & furniture"},
	{"leroy merlin", "Home & furniture"},
	{"castorama", "Home & furniture"},
	{"but", "Home & furniture"},
	{"conforama", "Home & furniture"},
	{"brico depot", "Home & furniture"},
	{"maisons du monde", "Home & furniture"},
	{"action", "Home & furniture"},

	{"sephora", "Beauty & care"},
	{"marionnaud", "Beauty & care"},
	{"nocibe", "Beauty & care"},
	{"yves rocher", "Beauty & care"},
	{"coiffeur", "Beauty & care"},
	{"institut de beaute", "Beauty & care"},

	{"amazon", "Shopping"},
	{"cdiscount", "Shopping"},
	{"aliexpress", "Shopping"},

	// --- Savings & Investments ---------------------------------------------
	{"perin", "Investments"},
	{"plan epargne retraite", "Investments"},
	{"assurance vie", "Investments"},
	{"pea", "Investments"},
	{"binance", "Crypto"},
	{"coinbase", "Crypto"},
	{"kraken", "Crypto"},

	// --- Family & Children ------------------------------------------------
	{"creche", "Childcare"},
	{"assistante maternelle", "Childcare"},
	{"cantine", "School fees"},
	{"centre aere", "Children's activities"},

	// --- Taxes ----------------------------------------------------------
	{"dgfip", "Income tax"},
	{"impot sur le revenu", "Income tax"},
	{"tresor public", "Income tax"},
	{"taxe fonciere", "Property tax"},
	{"taxe d habitation", "Property tax"},
	{"amende", "Fines"},
	{"anpr", "Fines"},

	// --- Gifts & Donations ------------------------------------------------
	{"unicef", "Charity & donations"},
	{"croix rouge", "Charity & donations"},
	{"secours populaire", "Charity & donations"},
	{"restos du coeur", "Charity & donations"},

	// --- Pets -----------------------------------------------------------
	{"veterinaire", "Veterinary"},
	{"clinique veterinaire", "Veterinary"},
	{"wanimo", "Pet food & supplies"},
	{"zooplus", "Pet food & supplies"},
}

// normalizeForMatching lowercases and replaces every run of non
// alphanumeric characters with a single space, then pads the result with a
// leading/trailing space — deliberately more aggressive than the package's
// own normalizeLabel (bare lowercase+trim, used where reconciliation needs
// to preserve exact structure). This one exists purely so a keyword can be
// matched at word boundaries regardless of the punctuation bank labels
// throw around it ("E.LECLERC", "PAYLI2441535/", "BAILLEUL2378/ BRICO
// DEPOT") without a short keyword like "but" spuriously matching inside an
// unrelated longer word.
var nonAlnumRun = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeForMatching(label string) string {
	lower := strings.ToLower(label)
	collapsed := nonAlnumRun.ReplaceAllString(lower, " ")
	return " " + strings.TrimSpace(collapsed) + " "
}

// suggestCategoryID returns the standard category id merchantDictionary
// maps label to, or nil if nothing matches. When several keywords match,
// the longest (most specific — e.g. "e leclerc" over a hypothetical
// shorter overlapping entry) wins.
func suggestCategoryID(label string, standardCategoryIDByName map[string]uuid.UUID) *uuid.UUID {
	normalized := normalizeForMatching(label)
	var bestKeyword string
	var bestID uuid.UUID
	found := false

	for _, rule := range merchantDictionary {
		needle := " " + rule.keyword + " "
		if !strings.Contains(normalized, needle) {
			continue
		}
		if len(rule.keyword) <= len(bestKeyword) {
			continue
		}
		id, ok := standardCategoryIDByName[rule.category]
		if !ok {
			continue
		}
		bestKeyword = rule.keyword
		bestID = id
		found = true
	}

	if !found {
		return nil
	}
	return &bestID
}

// AutoCategorizeUseCase assigns a category to an account's currently
// uncategorized operations — see the package doc comment above for the two
// mechanisms it tries, in order. Purely additive: never touches an
// operation that already has a category (whether set by a user or a
// previous run of this), so re-running it after new operations arrive is
// always safe.
type AutoCategorizeUseCase struct {
	operations ports.OperationRepository
	categories ports.CategoryRepository
}

func NewAutoCategorizeUseCase(operations ports.OperationRepository, categories ports.CategoryRepository) *AutoCategorizeUseCase {
	return &AutoCategorizeUseCase{operations: operations, categories: categories}
}

// CategorizeAccount returns how many of accountID's operations it assigned
// a category to. ownerUserID is only used to resolve the visible standard
// category set (see CategoryRepository.ListVisibleTo) — a collaborator
// triggering this on a shared account still resolves the owner's
// categories, same as every other category lookup in this codebase.
func (uc *AutoCategorizeUseCase) CategorizeAccount(ctx context.Context, accountID, ownerUserID uuid.UUID) (int, error) {
	ops, err := uc.operations.ListForAccount(ctx, accountID, nil, nil)
	if err != nil {
		return 0, err
	}

	var uncategorized []entities.Operation
	labelSet := map[string]struct{}{}
	for _, op := range ops {
		if op.CategoryID != nil {
			continue
		}
		uncategorized = append(uncategorized, op)
		labelSet[normalizeLabel(op.Label)] = struct{}{}
	}
	if len(uncategorized) == 0 {
		return 0, nil
	}

	labels := make([]string, 0, len(labelSet))
	for l := range labelSet {
		labels = append(labels, l)
	}
	ownTop, crowdTop, err := uc.labelHistory(ctx, labels, ownerUserID)
	if err != nil {
		return 0, err
	}

	categories, err := uc.categories.ListVisibleTo(ctx, ownerUserID)
	if err != nil {
		return 0, err
	}
	standardCategoryIDByName := make(map[string]uuid.UUID, len(categories))
	for _, c := range categories {
		if c.UserID == nil {
			standardCategoryIDByName[c.Name] = c.ID
		}
	}

	count := 0
	for i := range uncategorized {
		op := &uncategorized[i]
		key := normalizeLabel(op.Label)

		var categoryID *uuid.UUID
		if id, ok := ownTop[key]; ok {
			categoryID = &id
		} else if id, ok := crowdTop[key]; ok {
			categoryID = &id
		} else if id := suggestCategoryID(op.Label, standardCategoryIDByName); id != nil {
			categoryID = id
		}
		if categoryID == nil {
			continue
		}

		op.CategoryID = categoryID
		if _, err := uc.operations.Update(ctx, op); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// labelHistory turns CategoryCountsForLabels' flat rows into two lookups
// per normalized label: ownTop (ownerUserID's own most common past choice,
// if any) and crowdTop (the system-wide most common choice across every
// user, ownerUserID's own included). Ties broken arbitrarily but
// deterministically-ish (map iteration order) — rare enough in practice
// (it means two categories were picked exactly the same number of times for
// the exact same label) not to be worth a tiebreaker rule.
func (uc *AutoCategorizeUseCase) labelHistory(ctx context.Context, normalizedLabels []string, ownerUserID uuid.UUID) (ownTop, crowdTop map[string]uuid.UUID, err error) {
	counts, err := uc.operations.CategoryCountsForLabels(ctx, normalizedLabels)
	if err != nil {
		return nil, nil, err
	}

	ownBest := map[string]int{}
	ownTop = map[string]uuid.UUID{}
	for _, c := range counts {
		if c.OwnerUserID == ownerUserID && c.Count > ownBest[c.NormalizedLabel] {
			ownBest[c.NormalizedLabel] = c.Count
			ownTop[c.NormalizedLabel] = c.CategoryID
		}
	}

	// crowdTop sums each category's count across every owner sharing this
	// label (not just the most frequent single owner), so a category three
	// different users each picked once outranks one only a single user
	// picked twice.
	crowdSums := map[string]map[uuid.UUID]int{}
	for _, c := range counts {
		byCategory := crowdSums[c.NormalizedLabel]
		if byCategory == nil {
			byCategory = map[uuid.UUID]int{}
			crowdSums[c.NormalizedLabel] = byCategory
		}
		byCategory[c.CategoryID] += c.Count
	}
	crowdBest := map[string]int{}
	crowdTop = map[string]uuid.UUID{}
	for label, byCategory := range crowdSums {
		for categoryID, n := range byCategory {
			if n > crowdBest[label] {
				crowdBest[label] = n
				crowdTop[label] = categoryID
			}
		}
	}
	return ownTop, crowdTop, nil
}
