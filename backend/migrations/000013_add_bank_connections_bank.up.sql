-- Which bank aggregator backend (woob module) this connection talks to -
-- only 'cm' (Credit Mutuel) is supported today (validated in
-- ConnectBankUseCase.Connect), but every connection records it explicitly
-- rather than assuming, so a second bank later doesn't need a backfill.
-- Backfill any pre-existing rows to 'cm' (the only bank this ever was),
-- then drop the default so every future insert must state it explicitly -
-- mirrors how the Go code always sets it now.
ALTER TABLE bank_connections ADD COLUMN bank TEXT NOT NULL DEFAULT 'cm';
ALTER TABLE bank_connections ALTER COLUMN bank DROP DEFAULT;
