# Small Workflow Guide:

* always use conventional commits (conventional commit messages)

* for non-small changes, always commit changes in parts instead of committing several hundred lines of changes and multiple file changes in one commit. (if possible or ideal) and make sure to also push them.

* for non-small changes, always use the gh cli to execute CI jobs, and view and watch their output and progress to ensure changes come out clean. (run "gh help" for more info) you must push your changes first before using the gh cli to interact with the ci actions.

* for mostly all changes, once comfortable with their state run the taskfile task "ops" in order to do linting and ensure that all your changes are clean, alternatively run the scripts in scripts/ops directly if needed.

* make sure to use the build task to build after you make your changes to ensure that the binaries compile properly.

* make sure to do testing of your changes before finalizing anything, to ensure that all works fine.

* make sure to update docs if they are outdated.

* make sure to always read and or briefly read the codebase and the codebases git history for your relevant (planned) changes properly before making changes in order to stay dynamic.