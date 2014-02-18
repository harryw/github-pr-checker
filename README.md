Github PR Checker
=================

This is a simple command-line app to check the age of GitHub pull requests.

If a PR is older than a given threshold and has a configured HipChat hook, a notification is sent to the specified
HipChat room.

#Usage

The utility accepts a number of command-line options:

```bash
-room <room>                Exclusively notify the specified HipChat room
-repo-api-token <token>     GitHub API token with 'repo' scope
-hook-api-token <token>     GitHub API token with 'read:repo_hook' scope
-days <days>                Number of days old the PR may be before considering it old
```


