#!/bin/bash
set -e
cd /var/tmp/m7old
unset SUBUTAI_DATABASE_URL SUBUTAI_TEST_DATABASE_URL
export CROMWELL_DATABASE_URL="postgres://postgres@localhost:54329/m7_old?sslmode=disable"
db() { psql "postgres://postgres@localhost:54329/m7_old" -Atc "$1"; }
echo "--- subutai status (a CLI command in the old project):"
/var/tmp/m7/subutai status
echo "--- a commit, through the refreshed hook:"
echo "Through the new hook." >> docs/greet/time/design.md
git commit -qam "edit via the refreshed hook"; sleep 2
db "select kind, actor, occurred_at from audit_events order by occurred_at desc limit 1"
echo "--- a commit with hooks off, then the Cromwell binary's hook command by hand:"
echo "Through the old binary." >> docs/greet/time/design.md
git -c core.hooksPath=/dev/null commit -qam "edit, hook skipped"
/var/tmp/m7/cromwell hook post-commit --repo /var/tmp/m7old; sleep 2
db "select kind, actor, occurred_at from audit_events order by occurred_at desc limit 1"
echo "--- file hash vs recorded hash:"
sha256sum docs/greet/time/design.md | cut -d' ' -f1
db "select content_hash from documents where path='docs/greet/time/design.md'"
echo "--- the Cromwell binary's status, over the kept socket name:"
/var/tmp/m7/cromwell status | head -3
echo "--- audit rows from before the rename are as recorded:"
db "select kind, actor from audit_events where kind in ('initiative.created','feature.created','document.registered') order by occurred_at"
