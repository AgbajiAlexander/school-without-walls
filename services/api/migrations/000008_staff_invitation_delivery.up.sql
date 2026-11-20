-- Existing verification and redemption queries accept only PENDING tokens.
ALTER TYPE invitation_status ADD VALUE IF NOT EXISTS 'AWAITING_DELIVERY';
