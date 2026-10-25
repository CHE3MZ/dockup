# Local CI/CD Jobs via `container`

These CI/CD jobs use the `container` CLI tool to manage their execution environments.

Both Windows and macOS provide a CLI tool named `container`. Although these tools target different platforms and use different backends, they expose a similar API:

- **Windows:** `container` (also known as `wslc`)
  - you must have wsl installed in order to have this tool
- **macOS:** `container` (Apple's containerization tool)
  - you can install container via brew, `brew install container`

Using this common API, these CI/CD jobs start the container engine for the current platform, run the required tasks, and stop the engine when the jobs are complete.

## Why?

These container jobs are for heavier CI tasks that might need sandboxing,
a linux environment, or are generally not ideal to run on the host OS directly.
Note that container jobs, especially temporary container jobs (like example.sh)
will take much longer to setup and finish in comparison to running a script 
directly on the host OS, so reserve these container jobs for tasks that are 
non-lightweight or genuinely need containerization before running.

## Example

See [`jobs/example.sh`](jobs/example.sh) (an example of a temporary container job which clears itself post execution) for reference.

## Note

> Linux does not currently provide a compatible CLI tool named `container`, so > these jobs do not natively support Linux.
> Ironically, the jobs themselves run inside Linux containers. Native Linux 
> support may be added in the future.
