*( this TODO list should be picked up later not now , we should focus on testing and improving the codebase as is right now first, before going through this TODO list, the items here are not very significant so they shall come last in development) use **✔ for done** , **✗ for not done** , **WIP for work-in-progress** (currently doing).*

---

- [✔] **add a restore command to restore the distro to its default state. this should help fix any potentially unwanted modifications having been made to distro get rest, command name should be "dockup restore" and it should have a lightweight solution to restoring stuff, it should not be heavy. --this feature is important and should be considered at some point when the rest of the stuff is done.**

- [✗] **move the "installed" variable from .dockup/config.json to .dockup/installation.json**

- [✗] **add a browser web UI using Vue 3 + Vite + Vue Vapor feature , bun for build test , net/http + embed for bridge , served to host at localhost:8060 ( experimental - should not be implemented yet until everything works. )**

- [✗] **writing a proper, simplistic readme.md , and writing full documentation into docs/docs**

- [✗] **small change : using the assets/icon-black.png or assets/icon-white.png as the exe build icon.**

- [✗] **creating a CD job for auto tag creator and releaser to build and release windows binaries and create a new tag and assign the binary (zipped as tar.gz ) to the tag.**
