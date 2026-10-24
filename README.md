<div align="center">

# ⇄ prwatch

**see which of your open github PRs need you.**<br>
one command, one api call, and the ones waiting on you go to the top.

[![ci](https://img.shields.io/github/actions/workflow/status/Dem1241/prwatch/ci.yml?branch=main&style=flat-square&label=ci)](https://github.com/Dem1241/prwatch/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-f5c2e7?style=flat-square)](LICENSE)

<img src="assets/demo.gif" alt="prwatch running on made-up data: four rows under your move, including a PR with changes requested and a discussion with a fresh maintainer reply, and five rows waiting on maintainers, one of them flagged stale after 19 days" width="860">

</div>

## why

once you have twenty PRs open across fifteen repos, github's notifications
stop being useful. a maintainer asks a question on one, a CLA bot fails on
another, a third has been sitting unreviewed for three weeks, and all of it
looks the same in the inbox.

prwatch answers one question for every open PR, and for any issue or
discussion you're following: is it my move, or theirs?

## try it

you need [`gh`](https://cli.github.com) logged in, or a `GITHUB_TOKEN` in the
environment.

```sh
go install github.com/Dem1241/prwatch@latest
prwatch
```

or with nix:

```sh
nix run github:Dem1241/prwatch
```

## use it

```sh
prwatch                 # print once
prwatch -watch 5m       # redraw every 5 minutes, desktop notification when something becomes your move
prwatch -json           # rows as json, for scripts
prwatch -stale 7        # flag PRs that have waited on others for a week (default 14)
prwatch -user someone   # someone else's PRs
prwatch -demo           # made-up data, no login needed
```

the gif above is recorded from `-demo` with [vhs](https://github.com/charmbracelet/vhs): `vhs assets/demo.tape`.

notifications go through `notify-send` when it's on your `PATH`. turn them off
with `-no-notify`. nothing is sent on the first refresh, only for things that
change while it runs.

### tracking issues and discussions

open PRs are found automatically. for issues and discussions, list them in
`~/.config/prwatch/tracked.txt` (or pass `-tracked path`):

```
# kind owner/repo number [seen]
disc  duckdb/duckdb 27705
issue golang-migrate/migrate 1439
issue duckdb/duckdb 27826 2026-10-24T18:38:51Z
```

`seen` is optional. replies at or before that time count as handled, which
helps when you answered somewhere prwatch can't see, like in a linked PR.

## how it decides

it compares your latest activity on each thread (commits, comments, reviews)
with the latest activity from any other human. bots like codecov, CLA
checkers and `*[bot]` accounts are ignored.

| status | your move? |
| --- | --- |
| CI failing | yes |
| changes requested | yes, when the request is newer than your last activity |
| reply waiting | yes, someone wrote after you |
| still a draft | yes |
| approved, awaiting merge | no |
| no review yet / waiting on them | no, shown as stale in red after `-stale` days |

the PR list, merge counts and every tracked thread come back in a single
graphql request, so `-watch 1m` stays well inside the rate limit.

## license

MIT
