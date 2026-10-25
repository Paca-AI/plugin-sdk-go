#!/bin/sh
# Verifies the Git provider conformance suite catches known-bad providers:
# every TestMutant_* must FAIL. Exit status is non-zero if any mutant survives.
cd "$(dirname "$0")/.." || exit 1
out=$(go test -tags mutants ./gitprovider/fakeprovider/ -run 'TestMutant' -v 2>&1)
survived=0
for name in $(echo "$out" | grep -o '^=== RUN   TestMutant_[A-Za-z]*$' | awk '{print $3}'); do
  if echo "$out" | grep -q -- "--- FAIL: $name "; then
    echo "caught   $name"
  else
    echo "SURVIVED $name"; survived=1
  fi
done
exit $survived
