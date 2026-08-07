-- SPDX-License-Identifier: Apache-2.0
ALTER TABLE risk_scores
    DROP COLUMN exceedance,
    DROP COLUMN explanation;
