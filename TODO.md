- [x] setup progress honesty (all fixed):
  - path example now `D:/WSL/Dockup` (was `D:/WSL`, inviting a shared root)
  - debian download is one live line (was a static `0/N` line plus the mover)
  - import/install/configure/test show a live elapsed spinner + `done`
    (was fake `0%` stuck for minutes, then a jump to `100%`)
- [x] bare `docker` works: the default `docker_engine` pipe is mirrored
  while dockup runs (skipped when Docker Desktop already holds it)
- [x] foreground Ctrl+C: SIGTERM handled too, streaming bridges are reaped
  on shutdown, and `dockup shutdown` stops a stuck foreground
  (dockup.exe PIDs only, name-verified against PID reuse)
- [x] CLI polish: startup reports the mirror state, no duplicate or fake
  progress lines remain
