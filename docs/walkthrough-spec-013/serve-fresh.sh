#!/bin/bash
cd /var/tmp/m7demo
export SUBUTAI_DATABASE_URL="postgres://postgres@localhost:54329/m7_fresh?sslmode=disable" ANTHROPIC_API_KEY=unused
exec env -u CROMWELL_TEST_DATABASE_URL /var/tmp/m7/subutai serve > /var/tmp/m7/serve-fresh.log 2>&1
