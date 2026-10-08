- [x] add dependabot to the project.
- [x] explore and potentially switch to debian:slim — DECLINED (slim rootfs manifest has no systemd/systemd-sysv/procps; only libsystemd0. No PID 1, no systemctl, no ps. A 42% smaller download (27 vs 46 MB) is not worth re-platforming the systemd-first design onto an undocumented transitive dep).

- [x] fix CLI text rendering issues and line seperation issues.
  - details:
    - issue 1: ctrl + C before dockup starts results in these two lines rendering as one line:
      ⠧ warming up the dockup distro...shutting down dockup...
      it should be 
      ⠧ warming up the dockup distro...
      shutting down dockup...
      instead
    - issue 2: the example text in dockup help (the main help text) is not needed
      remove these lines:
      Examples:
        dockup setup
        docker -H npipe:////./pipe/dockup_engine run --rm hello-world
        dockup ssh -- journalctl -u docker.service --no-pager -n 30
      as it is practically not really needed so we can just get rid of it