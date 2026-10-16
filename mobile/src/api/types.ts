// Mirrors frontend/src/api/types.ts — kept as a hand-copied subset for now
// (no shared package between the two clients yet); extend as more screens
// get ported.

export type User = {
  id: string;
  email: string | null;
  display_name: string;
  has_password: boolean;
  has_robine_link: boolean;
  has_google_link: boolean;
  /** False until the address is confirmed through the emailed link (or a
   * provider vouched for it). Sharing invites need a verified address. */
  email_verified: boolean;
};

export type AuthConfig = {
  google_enabled: boolean;
  robine_enabled: boolean;
  apple_enabled: boolean;
};

export type Account = {
  id: string;
  name: string;
  currency: string;
  created_at: string;
  owner_display_name: string | null;
};

export type AmountMode = 'single' | 'debit_credit' | 'debit_credit_signed';

export type ColumnMapping = {
  date: string;
  label: string;
  amount?: string | null;
  debit?: string | null;
  credit?: string | null;
  reference?: string | null;
};

export type BankProfile = {
  id: string;
  global: boolean;
  name: string;
  delimiter: string;
  encoding: string;
  date_format: string;
  decimal_separator: string;
  amount_mode: AmountMode;
  column_mapping: ColumnMapping;
  created_at: string;
  updated_at: string;
};

export type ImportBatch = {
  id: string;
  filename: string;
  imported_at: string;
  row_count: number;
  imported_count: number;
  duplicate_count: number;
  errors: string[];
};

export type OperationSplit = { id: string; category_id: string; amount: string };

export type Operation = {
  id: string;
  date: string;
  label: string;
  amount: string;
  balance_after: string | null;
  raw_reference: string | null;
  category_id: string | null;
  splits: OperationSplit[];
};

export type OperationsPage = { operations: Operation[]; next_cursor: string | null };

export type PrevisionEntry = {
  date: string;
  amount: string;
  kind: 'actual' | 'forecast' | 'adjustment';
  operation_id: string | null;
  planned_occurrence_id: string | null;
  planned_item_id: string | null;
  balance_checkpoint_id: string | null;
  running_balance: string;
};

export type Previsions = {
  opening_balance: string;
  closing_balance: string;
  entries: PrevisionEntry[];
};

export type OccurrenceStatus = 'forecast' | 'reconciled' | 'skipped';

export type RecurringSuggestion = {
  label: string;
  kind: PlannedItemKind;
  suggested_amount: string;
  anchor_date: string;
  recurrence_type: RecurrenceType;
  interval_weeks: number | null;
  day_of_month: number | null;
  category_id: string | null;
  occurrence_count: number;
  last_operation_id: string;
};

export type Occurrence = {
  id: string;
  due_date: string;
  expected_amount: string;
  status: OccurrenceStatus;
  reconciled_operation_id: string | null;
  /** Only set for a category-mode budget's occurrence — the live sum of
   * every operation in its category over the period. null for a label-mode
   * item, which uses reconciled_operation_id instead. */
  actual_amount: string | null;
};

export type OccurrenceSummary = Occurrence & { planned_item_id: string };

export type PlannedItemKind = 'charge' | 'income';
export type RecurrenceType = 'one_off' | 'perpetual' | 'fixed_count' | 'every_x_weeks';
/** 'label': matches one operation to one occurrence by exact label (a bill,
 * a subscription). 'category': a budget envelope — every operation in the
 * category over a period is summed into that period's occurrence instead of
 * matched one at a time (see Occurrence.actual_amount). */
export type PlannedItemMatchMode = 'label' | 'category';

export type PlannedItem = {
  id: string;
  account_id: string;
  kind: PlannedItemKind;
  label: string | null;
  match_mode: PlannedItemMatchMode;
  amount: string;
  start_date: string;
  recurrence_type: RecurrenceType;
  occurrence_count: number | null;
  interval_weeks: number | null;
  day_of_month: number | null;
  active: boolean;
  notes: string | null;
  category_id: string | null;
};

export type BalanceCheckpoint = {
  id: string;
  as_of_date: string;
  balance: string;
  note: string | null;
  created_at: string;
};

export type Category = {
  id: string;
  parent_id: string | null;
  name: string;
  is_standard: boolean;
  editable: boolean;
};

export type Comment = {
  id: string;
  user_id: string;
  body: string;
  created_at: string;
};

export type OperationSummary = {
  id: string;
  date: string;
  label: string;
  amount: string;
};

export type ReconciliationSuggestion = {
  occurrence: OccurrenceSummary;
  operation: OperationSummary;
};

export type VersionInfo = {
  commit: string;
  branch: string;
  build_date: string;
};

export type CollaboratorStatus = 'pending' | 'accepted' | 'revoked';

export type Collaborator = {
  id: string;
  token: string;
  status: CollaboratorStatus;
  invited_email: string;
  collaborator_name: string | null;
  collaborator_email: string | null;
  created_at: string;
  accepted_at: string | null;
};

export type InviteInfo = { owner_display_name: string; status: CollaboratorStatus };

export type BankSyncResult = { synced_accounts: number; created_accounts: number };

/** `available` is false when bank sync is closed to this account (it is
 * limited to an allow-list server-side) — the UI hides the feature then. */
export type BankConnectionStatus = { connected: boolean; available: boolean; bank?: string };

/** An app (Claude, another MCP client) the user authorized through the
 * backend's OAuth consent page — see GET /api/oauth/grants. */
export type ConnectedApp = {
  id: string;
  client_name: string;
  created_at: string;
  last_used_at: string;
};
