#!/bin/bash
cd /var/tmp/m7old
unset CROMWELL_DATABASE_URL SUBUTAI_TEST_DATABASE_URL CROMWELL_TEST_DATABASE_URL
export SUBUTAI_DATABASE_URL="postgres://postgres@localhost:54329/m7_old?sslmode=disable" ANTHROPIC_API_KEY=unused
stop() { kill $(pgrep -f "^/var/tmp/m7/subutai serve" | while read p; do [ "$(readlink /proc/$p/cwd)" = /var/tmp/m7old ] && echo $p; done) 2>/dev/null; sleep 1; }
run() { setsid /var/tmp/m7/subutai serve > /var/tmp/m7/serve-$1.log 2>&1 < /dev/null & sleep 2; echo "--- serve, $2:"; cat /var/tmp/m7/serve-$1.log; }
sed -i 's/url_env: SUBUTAI_DATABASE_URL/url_env: CROMWELL_DATABASE_URL/' .subutai/config.yaml
run renamed-a "folder renamed, SUBUTAI_DATABASE_URL set, config.yaml still naming CROMWELL_DATABASE_URL"
/var/tmp/m7/subutai status | head -2
stop
git checkout -q .subutai/config.yaml
run renamed-b "config.yaml updated too"
/var/tmp/m7/subutai status | head -2
echo "--- a commit through the hook after the rename:"
n0=$(psql "postgres://postgres@localhost:54329/m7_old" -Atc "select count(*) from audit_events where kind='document.indexed'")
echo "After the rename." >> docs/greet/time/design.md; git commit -qam "edit after rename"; sleep 2
n1=$(psql "postgres://postgres@localhost:54329/m7_old" -Atc "select count(*) from audit_events where kind='document.indexed'")
echo "document.indexed rows: $n0 -> $n1"
ls .subutai/run
