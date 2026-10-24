DROP INDEX IF EXISTS idx_reservation_confirmations_call;
ALTER TABLE reservation_confirmations
    DROP COLUMN IF EXISTS next_call_at,
    DROP COLUMN IF EXISTS calls_made,
    DROP COLUMN IF EXISTS call_retry_minutes,
    DROP COLUMN IF EXISTS call_retries,
    DROP COLUMN IF EXISTS call_window_end_minute,
    DROP COLUMN IF EXISTS call_window_start_minute;
ALTER TABLE confirmation_settings
    DROP CONSTRAINT IF EXISTS ck_confirmation_settings_call_window,
    DROP COLUMN IF EXISTS call_retry_minutes,
    DROP COLUMN IF EXISTS call_retries,
    DROP COLUMN IF EXISTS call_window_end_minute,
    DROP COLUMN IF EXISTS call_window_start_minute;
