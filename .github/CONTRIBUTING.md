# Contributing to dockup

Thanks for helping out. Full guide:
<https://github.com/CHE3MZ/dockup/tree/main/docs/docs/contributing.md>

- **Testing.** Most testing runs in the cloud through this project's GitHub
  Actions workflows, though a local run is fine when you can do one. Test your
  changes, and don't open a PR for untested or unreviewed work. Push only
  after the build and the linters are clean; that keeps the code easy to
  maintain and review.
- **Short commit messages.** Keep them short and readable. Skip "updated
  something" and "fixed something"; a one-line summary of what the commit
  adds or removes is enough.
- **Bug checks.** Run the linters and other checkers as you change things.
  `scripts\ops\go-bugcheck.sh` and `scripts\ops\go-seccheck.sh` write their
  output to the logs folder. Install whatever they need first.

  *Running these linters is a must for big changes.*

- **Tests.** Run and add tests when possible or needed.
- **Docs.** Document your changes in `docs/docs` when possible.
- **Comments.** Add comments next to your changes when possible.
- **Frozen strings.** CI greps them, so never reword: the setup banner flow,
  `dockup has not been setup yet run "dockup setup" to set it up.`,
  `starting...`, `test results :`, `autostart: on/off`,
  `STATUS/AUTOSTART/INSTALLED/SIZE/MEMORY`, `read-only`, `ok: autostart on`,
  `dry run complete`.
- **Green before review.** Most workflows run automatically, so check that
  they all pass before you submit. If one is red, find out why.
