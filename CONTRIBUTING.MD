# Guide to Contributing

### Pushing Changes to `dev`
- Active development happens on the `dev` branch; `main` mirrors upstream and only receives merges from `dev`. New changes should be pushed to a new branch, and then a pull request is made to merge that new branch back into `dev` (see `AGENTS.md` for the full workflow).
- Steps:
	1. Create a new local branch:
		* `git switch -c <new_branch_name>`
	2. Confirm the new branch was created:
		* `git branch -a`
	3. Confirm the upstream branch you'll be committing to and make a push:
		* `git push --set-upstream origin <new_branch_name>`
	4. Open a pull request targeting the `dev` branch.
