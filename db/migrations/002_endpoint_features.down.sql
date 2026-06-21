ALTER TABLE endpoints DROP COLUMN IF EXISTS secret;
ALTER TABLE endpoints DROP COLUMN IF EXISTS slack_webhook_url;
ALTER TABLE endpoints DROP COLUMN IF EXISTS rate_limit_per_second;
ALTER TABLE endpoints DROP COLUMN IF EXISTS rate_limit_burst;
