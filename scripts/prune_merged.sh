#!/bin/sh
# Tidies up the work branches and worktrees that pile up in this repository (eng/*, spec/* and ui/*). It
# only ever touches local branches that are already fully merged into main, and their worktrees under .worktrees/ when
# those have no changes of their own (build output and node_modules, which git ignores, do not count). Unmerged
# branches, dirty worktrees, main and the branch you are on are always left alone. Remote branches are not touched.
#
# ow/* and wip/* branches are left out on purpose: a builder that just started from main looks "merged" too.
#
# By default it only prints what it would do. Pass --apply to really do it.
#
#   scripts/prune_merged.sh            show what would be removed
#   scripts/prune_merged.sh --apply    remove it
set -e
cd "$(dirname "$0")/.."

apply=0
[ "$1" = "--apply" ] && apply=1

# Runs a command when --apply was given, otherwise just prints it.
run() {
  if [ $apply -eq 1 ]; then "$@"; else echo "would run: $*"; fi
}

current=$(git rev-parse --abbrev-ref HEAD)

# Every merged work branch, one per line.
for branch in $(git branch --merged main --format='%(refname:short)' | grep -E '^(eng|spec|ui)/'); do
  [ "$branch" = "$current" ] && continue

  # The worktree that has this branch checked out, if any.
  wt=$(git worktree list --porcelain | awk -v b="refs/heads/$branch" '
    /^worktree / { path = substr($0, 10) }
    $0 == "branch " b { print path }')

  if [ -n "$wt" ]; then
    case "$wt" in
      */.worktrees/*) ;;
      *) echo "skip $branch: checked out at $wt (outside .worktrees/)"; continue ;;
    esac
    # Tracked changes or untracked files mean someone is still working there.
    if [ -n "$(git -C "$wt" status --porcelain)" ]; then
      echo "skip $branch: worktree $wt has uncommitted changes"
      continue
    fi
    # --force only lets git drop the ignored build output; the check above already made sure nothing else is there.
    run git worktree remove --force "$wt"
  fi
  run git branch -d "$branch"
done

run git worktree prune
