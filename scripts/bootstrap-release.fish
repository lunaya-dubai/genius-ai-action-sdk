#!/usr/bin/env fish
# Bootstrap: tidy, test, commit, tag, push the public action SDK.
set -g ROOT (dirname (status filename))
cd $ROOT

go mod tidy
or begin; echo 'go mod tidy failed'; status 1; end

go test ./...
or begin; echo 'go test failed'; status 1; end

git add -A
git status -sb

if test (count (git status --porcelain)) -eq 0
    echo 'nothing to commit'
else
    git commit -m "$(cat <<'EOF'
Initial public Genius AI action SDK (action + s3).

Extracted from lunaya-flow-runtime so action authors depend on a public
module instead of the private runtime repo.
EOF
)"
end

git tag -f v0.1.0
git push -u origin main
git push origin v0.1.0
echo 'SDK seeded and tagged v0.1.0'
