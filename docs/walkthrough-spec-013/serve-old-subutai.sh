#!/bin/bash
# Serve the Cromwell-made project with the Subutai binary, old variable names only.
cd /var/tmp/m7old
unset SUBUTAI_DATABASE_URL SUBUTAI_TEST_DATABASE_URL
export CROMWELL_DATABASE_URL="postgres://postgres@localhost:54329/m7_old?sslmode=disable" ANTHROPIC_API_KEY=unused
exec /var/tmp/m7/subutai serve > /var/tmp/m7/serve-old-subutai.log 2>&1 < /dev/null
