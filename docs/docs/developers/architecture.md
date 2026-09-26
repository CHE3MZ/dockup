# Architecture

One static Windows binary, one disposable distro, one raw byte stream.

```text
Windows side (dockup.exe, Go, single static binary)
─────────────────────────────────────────────────────
docker.exe (user-installed, NOT bundled)
   │  npipe:////./pipe/dockup_engine (HTTP-over-pipe, hijack-capable)
   ▼
dockup foreground (`dockup`)  OR  dockup daemon child (`dockup __serve`)
   │  go-winio ListenPipe, per-connection goroutine
   ▼  wsl -d dockup -u root -- socat STDIO UNIX-CONNECT:/var/run/docker.sock
WSL side (distro "dockup", Debian 12, systemd=true)
─────────────────────────────────────────────────────
systemd (PID 1) → containerd.service → docker.service
   │  unix:///var/run/docker.sock
   ▼  socat bridge (one per client conn, spawned by Windows side)
containers
```

## Design decisions

- **Raw byte-copy, no HTTP parsing.** Hijacked streams (`run -it`, `exec`,
  `logs -f`, TTY resize) only survive a transparent proxy — anything that
  parses HTTP breaks them.
- **No TCP hop.** WSL2 NAT loopback can't reliably reach a distro-bound
  `127.0.0.1`, so the bridge speaks to the engine over socat `STDIO` per
  connection instead. TCP exists only as an opt-in `127.0.0.1` listener on
  the Windows side.
- **Message-mode pipe.** The listener uses `MessageMode: true` so a client's
  stdin close arrives server-side as EOF; the bridge then half-closes socat
  stdin and gives the container a 60s grace period to flush and exit.
- **Loud failures.** If the backend dies before producing a byte, the client
  gets HTTP 500 instead of a silent empty reply, and the daemon log records why.
- **`dockup_engine`, not `docker_engine`.** CI runners and user machines
  already hold Docker Desktop's pipe — hijacking it would fake a pass.
- **Debian rootfs, not nocloud.** The cloud `disk.raw` tarballs fail
  `wsl --import`; the debuerreotype rootfs tarballs (same bookworm content)
  import cleanly.
- **Single source of truth.** `installed` lives only in
  `%APPDATA%\dockup\state.json` (locked read-modify-write);
  `~/.dockup/config.json` holds user intent only. Old config files carrying
  the key still load — the key is ignored and dropped on next save.
- **systemd reboot.** One `wsl --terminate dockup` after writing `wsl.conf`
  so systemd becomes PID 1; every unit (re)start is preceded by
  `systemctl reset-failed` (start-limit throttling) and followed by a
  `WaitDaemon` poll (boot race). Install scripts end with `apt-get clean`
  because a dynamic VHDX only grows.
