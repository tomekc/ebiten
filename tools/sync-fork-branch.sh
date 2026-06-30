#!/usr/bin/env bash
#
# Copyright 2026 The Ebitengine Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

origin_remote=origin
fork_remote=fork
suffix=-3d
feature_branch=
push_stable=1
push_feature=0
force_stable=0
rebase_merges=0

usage() {
	cat <<EOF
Usage: ${0##*/} [options] [base-branch]

Synchronize a fork's stable branch with upstream, then rebase the suffixed
branch on top of it.

Examples:
  ${0##*/} 2.9
  ${0##*/} 2.10
  ${0##*/} --feature 2.9-3d-depth 2.9
  ${0##*/} --push-feature 2.9
  ${0##*/} --no-push 2.9

Options:
  --origin REMOTE       Upstream remote to fetch from (default: origin)
  --fork REMOTE         Fork remote to push to (default: fork)
  --suffix SUFFIX       Feature branch suffix (default: -3d)
  --feature BRANCH      Feature branch to rebase (default: BASE + SUFFIX)
  --rebase-merges       Preserve merge commits while rebasing
  --force-stable        Allow overwriting a divergent local or fork stable branch
  --push-feature        Push the rebased feature branch with --force-with-lease
  --no-push-stable      Do not push the synchronized stable branch to the fork
  --no-push             Rebase locally without pushing either branch to the fork
  -h, --help            Show this help

If base-branch is omitted and the current branch ends in the suffix, the base is
derived by stripping the suffix. For example, 2.9-3d implies base branch 2.9.

By default, the synchronized stable branch is pushed to the fork, but the
rebased suffixed branch is left local for testing. Use --push-feature after
testing, or push it manually.
EOF
}

die() {
	echo "error: $*" >&2
	exit 1
}

run() {
	printf '+'
	printf ' %q' "$@"
	printf '\n'
	"$@"
}

ref_exists() {
	git show-ref --verify --quiet "$1"
}

branch_exists() {
	git show-ref --verify --quiet "refs/heads/$1"
}

remote_branch_exists() {
	git ls-remote --exit-code --heads "$1" "$2" >/dev/null 2>&1
}

require_clean_worktree() {
	if [[ -n "$(git status --porcelain)" ]]; then
		die "worktree is not clean; commit or stash local changes before rebasing"
	fi
}

fetch_remote_branch() {
	local remote=$1
	local branch=$2

	run git fetch "$remote" "refs/heads/$branch:refs/remotes/$remote/$branch"
}

push_with_lease() {
	local remote=$1
	local local_ref=$2
	local remote_branch=$3
	local remote_tracking_ref="refs/remotes/$remote/$remote_branch"

	if ref_exists "$remote_tracking_ref"; then
		local expected
		expected=$(git rev-parse "$remote_tracking_ref")
		run git push --force-with-lease="refs/heads/$remote_branch:$expected" \
			"$remote" "$local_ref:refs/heads/$remote_branch"
	else
		run git push -u "$remote" "$local_ref:refs/heads/$remote_branch"
	fi
}

base_branch=

while (($#)); do
	case "$1" in
		--origin)
			shift
			origin_remote=${1:-}
			[[ -n "$origin_remote" ]] || die "--origin requires a remote name"
			;;
		--fork)
			shift
			fork_remote=${1:-}
			[[ -n "$fork_remote" ]] || die "--fork requires a remote name"
			;;
		--suffix)
			shift
			suffix=${1:-}
			[[ -n "$suffix" ]] || die "--suffix requires a non-empty suffix"
			;;
		--feature)
			shift
			feature_branch=${1:-}
			[[ -n "$feature_branch" ]] || die "--feature requires a branch name"
			;;
		--rebase-merges)
			rebase_merges=1
			;;
		--force-stable)
			force_stable=1
			;;
		--push-feature)
			push_feature=1
			;;
		--no-push-stable)
			push_stable=0
			;;
		--no-push)
			push_stable=0
			push_feature=0
			;;
		-h|--help)
			usage
			exit 0
			;;
		-*)
			die "unknown option: $1"
			;;
		*)
			[[ -z "$base_branch" ]] || die "multiple base branches specified"
			base_branch=$1
			;;
	esac
	shift
done

git rev-parse --git-dir >/dev/null

current_branch=$(git symbolic-ref --quiet --short HEAD) ||
	die "detached HEAD is not supported; check out a branch first"

if [[ -z "$base_branch" ]]; then
	if [[ "$current_branch" == *"$suffix" ]]; then
		base_branch=${current_branch%"$suffix"}
	else
		base_branch=$current_branch
	fi
fi

[[ -n "$feature_branch" ]] || feature_branch="${base_branch}${suffix}"
[[ "$base_branch" != "$feature_branch" ]] ||
	die "base and feature branch both resolved to $base_branch"

git remote get-url "$origin_remote" >/dev/null ||
	die "remote '$origin_remote' does not exist"
git remote get-url "$fork_remote" >/dev/null ||
	die "remote '$fork_remote' does not exist"

require_clean_worktree

echo "Syncing $fork_remote/$base_branch with $origin_remote/$base_branch"
echo "Rebasing $feature_branch onto $base_branch"

fetch_remote_branch "$origin_remote" "$base_branch"
ref_exists "refs/remotes/$origin_remote/$base_branch" ||
	die "upstream branch '$origin_remote/$base_branch' does not exist"

fork_base_exists=0
if remote_branch_exists "$fork_remote" "$base_branch"; then
	fetch_remote_branch "$fork_remote" "$base_branch"
	fork_base_exists=1
fi

fork_feature_exists=0
if remote_branch_exists "$fork_remote" "$feature_branch"; then
	fetch_remote_branch "$fork_remote" "$feature_branch"
	fork_feature_exists=1
fi

if ((fork_base_exists)) &&
	! git merge-base --is-ancestor "$fork_remote/$base_branch" "$origin_remote/$base_branch"; then
	if ((force_stable)); then
		echo "warning: $fork_remote/$base_branch diverged from $origin_remote/$base_branch; overwriting due to --force-stable" >&2
	else
		die "$fork_remote/$base_branch has commits not in $origin_remote/$base_branch; rerun with --force-stable to overwrite it"
	fi
fi

if branch_exists "$base_branch" &&
	! git merge-base --is-ancestor "$base_branch" "$origin_remote/$base_branch"; then
	if ((force_stable)); then
		echo "warning: local $base_branch diverged from $origin_remote/$base_branch; overwriting due to --force-stable" >&2
	else
		die "local $base_branch has commits not in $origin_remote/$base_branch; rerun with --force-stable to overwrite it"
	fi
fi

if [[ "$current_branch" == "$base_branch" ]]; then
	run git reset --hard "$origin_remote/$base_branch"
else
	if branch_exists "$base_branch"; then
		run git branch --force "$base_branch" "$origin_remote/$base_branch"
	else
		run git branch "$base_branch" "$origin_remote/$base_branch"
	fi
fi

if ((push_stable)); then
	if ((force_stable)); then
		push_with_lease "$fork_remote" "refs/remotes/$origin_remote/$base_branch" "$base_branch"
	else
		run git push "$fork_remote" "refs/remotes/$origin_remote/$base_branch:refs/heads/$base_branch"
	fi
fi

if ! branch_exists "$feature_branch"; then
	if ((fork_feature_exists)); then
		run git branch "$feature_branch" "$fork_remote/$feature_branch"
	else
		die "feature branch '$feature_branch' does not exist locally or on $fork_remote"
	fi
fi

run git switch "$feature_branch"

rebase_args=(git rebase)
if ((rebase_merges)); then
	rebase_args+=(--rebase-merges)
fi
rebase_args+=("$base_branch")
run "${rebase_args[@]}"

if ((push_feature)); then
	push_with_lease "$fork_remote" "refs/heads/$feature_branch" "$feature_branch"
fi

cat <<EOF

Done.
  Stable: $base_branch now matches $origin_remote/$base_branch
  Feature: $feature_branch rebased onto $base_branch
EOF

if ((push_stable)); then
	echo "  Pushed stable: $fork_remote/$base_branch"
else
	echo "  Pushed stable: no"
fi

if ((push_feature)); then
	echo "  Pushed feature: $fork_remote/$feature_branch"
else
	echo "  Pushed feature: no; test locally, then push with:"
	echo "    git push --force-with-lease $fork_remote $feature_branch"
fi
