# Revision :

due to the previous plans shortcomings and many other issues, a revision and reformatting of the whole
project architecture is needed, here is the new plan :

```
dockup setup installs a "nocloud" image from https://cloud.debian.org/images/cloud/bookworm/latest/
by default it install the adm64 one but if the flag --amd or --arm are passed it changes to that 
architecture install. the name "dockup" name will be assigned to the wsl distro name and once it 
is setup dockup will run wsl commands on that distro to install the docker daemon and configure it
via systemd to start up by default , the dockup distro will essentially act as a docker purpose only
distro so using systemd to autostart docker there is fine. then dockup will bridge itself with the
dockup distro and then will provide a docker daemon on windows that will be able to output and input
commands from a windows docker cli installation directly to the wsl and wsl outputs directly to windows.
under normal circumstances docker installed via normal install steps on a wsl distro and tcp'd to windows
fails to send output directly to the docker CLI unless the -i ( interactive ) flag is passed so dockup
must provide a more advanced daemon to the docker CLI in order to be able to directly send outputs from
an image or container that has been ran to the CLI without having to pass the interactive command each time.

an example of dockup setup :

## if dockup already is installed on wsl (a distro named dockup)

Warning : An installation of dockup already exists on WSL, do you wish to delete that installation and let
dockup re-install a new dockup instance on WSL ? [y/n]

## else

Setting up dockup :
    dockup will install and confiure a new instance on WSL under the name "dockup"
    proceed ? [y/n]

    >> y

    installing debian... (0/DOWNLOADSIZE_INMB mb) ## DOWNLOADSIZE_INMB should be the filesize being downloaded
    importing debian into WSL as "dockup"... (0%) ## show progress as %
    testing dockup on WSL... (please wait.)
    test results : success/failure ## if failure ask user "something went wrong , retry or abort ? [retry/abort]"
    installing docker... (0%) ## show progress as %
    configuring docker...  (0%) ## show progress as %
    testing docker... (0%) ## show progress as %
    test results : success/failure ## if failure ask user "something went wrong , retry or abort ? [retry/abort]"
    testing the docker daemon bridge... (0%) ## show progress as %
    test results : success/failure ## if failure ask user "something went wrong , retry or abort ? [retry/abort]"

    you're all good to go ! run "dockup" to start a foreground process or "dockup daemon start" to start a background
    daemon process.
```

```
other dockup commands and what they will do :

* dockup ## start dockup as a foreground process , note that if dockup daemon is running this cannot work and should
say that a dockup process is already running, same goes if another foreground process is found running it should tell
the user the action could not be done. also if dockup has not been setup yet it should say "dockup has not been setup yet
run "dockup setup" to set it up."
* dockup setup ## (already explained above)
* dockup uninstall ## (uninstalling dockup , must show confirmation box are you sure you want to uninstall your dockup
wsl distro ? [y/n] )
* dockup ps ## (show dockup status e.g stopped / running )

* dockup daemon ## note that if dockup foreground is running dockup daemon should show "dockup is already running as a 
foreground process, ctrl + C in order to use the dockup daemon."
- dockup daemon start ## start the background process , show health briefly to assure user dockup started , note that if dockup daemon is already running it should not do anything and instead should say "dockup is already running as a daemon process, 
run dockup restart to restart it." also note that if dockup is running as a foreground process via "dockup" it should alert
that "dockup is already running as a foreground process, ctrl + C it to stop it and re-run dockup start."
- dockup daemon stop ## stop the background process , show health briefly to assure user dockup stopped
- dockup daemon restart ## restart the background process , show health briefly to assure user dockup restart
- dockup daemon status ## see process health (brief health log e.g running , port health , errors etc.)
- dockup daemon log ## enter interactive log view to see the current full log, read only, ctrl + C to exit

* dockup shutdown ## stop all dockup processes including foreground and background daemon processes
* dockup doctor ## repair stale state and fix other issues and do other checks and preflight checks
* dockup version ## print "you're running the latest/dev version." for now.