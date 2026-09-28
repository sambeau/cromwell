#!/bin/bash
# Step 1: a project made and used with the Cromwell binary.
set -e
OLD=/var/tmp/m7/cromwell
rm -rf /var/tmp/m7old && mkdir -p /var/tmp/m7old && cd /var/tmp/m7old
git init -q && git config user.email sam@example.com && git config user.name sam
echo "# old" > README.md && git add -A && git commit -qm initial
export CROMWELL_DATABASE_URL="postgres://postgres@localhost:54329/m7_old?sslmode=disable" ANTHROPIC_API_KEY=unused
unset SUBUTAI_DATABASE_URL SUBUTAI_TEST_DATABASE_URL
$OLD init
printf '\nserver:\n  http: 127.0.0.1:8871\n  ui_actor: sam\n' >> .cromwell/config.yaml
git add -A && git commit -qm "cromwell init"
echo "--- the hook Cromwell installed:"; cat .git/hooks/post-commit
setsid $OLD serve > /var/tmp/m7/serve-old-cromwell.log 2>&1 < /dev/null &
sleep 2
$OLD initiative add greet --name "Greetings"
$OLD feature add greet/time --name "Tell the time"
mkdir -p docs/greet/time
printf -- "---\ntitle: Tell the time\ntype: design\nowner: greet/time\n---\n\n# Tell the time\n\nSay the time.\n" > docs/greet/time/design.md
git add -A && git commit -qm "design draft"
$OLD doc add docs/greet/time/design.md --type design --owner greet/time
ls .cromwell/run
pkill -f "/var/tmp/m7/cromwell serve"; sleep 1
echo "--- Cromwell server stopped"
