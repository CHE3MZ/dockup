# Contributing to dockup

Thanks for helping out. Full guide:
<https://github.com/CHE3MZ/dockup/tree/main/docs/docs/contributing.md>

- **Testing.** Testing must generally be done on the cloud via the github actions
  workflows that have been created for this project, local tests are preferred over
  cloud ones, but generally speaking most of the project's testing is done on the cloud,
  make sure to test all of your changes and report any errors etc. , it is generally
  recommended that you only push your changes once you have no runtime or pre-compliation
  linting errors, in order to ensure that the code remains high quality and possible to 
  maintain, please avoid opening pull requests for untested and or unreviewed changes,
  doing so makes things harder for me and or anyone else who is in charge of maintaining
  the project and reviewing/accepting PRs.
- **Short commit messages** please make sure to keep your commit messages short and well
  formatted, try to avoid using random commit messages like "updated something" or
  "fixed something", just a simple overview of what your commit adds or removes is enough
  I'm not asking for less nor more than that.
- **Bug Checks** make sure to run bug checkers linters and other tools as you're changing
  stuff to ensure that you don't write flawed code, you can use the scripts\ops\go-bugcheck.sh
  and the scripts\ops\go-seccheck.sh to quickly check your changes in the logs folder, make
  sure you have all the tools that the scripts demand installed before running them!

  *running these linters is a MUST if you are working on big changes.*
  
- **Tests:** make sure to run and add tests if possible or needed.
- **Docs:** make sure to document your changes into docs/docs if possible.
- **Comments:** make sure to add comments next to your changes if possible.
- **Frozen strings** (CI greps them): setup banner flow,
  `dockup has not been setup yet run "dockup setup" to set it up.`,
  `starting...`, `test results :`, `autostart: on/off`,
  `STATUS/AUTOSTART/INSTALLED/SIZE/MEMORY`, `read-only`, `ok: autostart on`,
  `dry run complete` — never reword.
- **Green before review:** make sure all the workflows are green before you
  submit your PR. most of them will run automatically so just check on them
  to see if everything is showing green, if not investigate why !
