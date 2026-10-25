set -eu

printf '\033[1mRUNNING JOBS...\033[0m\n'

for f in .ci/jobs/*.sh; do
    [ -f "$f" ] || continue
    [ "$f" = ".ci/jobs/example.sh" ] && continue

    printf '\033[32mRunning CI job: %s\033[0m\n' "$f"
    sh "$f"
done

printf '\033[1mDONE.\033[0m\n'