# Revision :

**github.com/CHE3MZ/dockup**

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
    dockup will install and configure a new instance on WSL under the name "dockup"
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

* dockup ## start dockup as a foreground process , you can do ctrl + C to stop dockup this way since its a forground process 
(IF you run it this way that is.), note that if dockup daemon is running this cannot work and should
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

```

# project structure :

* cli goes into cmd/dockup/main.go
* the codebase itself for everything goes into internal/ ideally as seperate folders for important things.
* build and test scripts etc. go into the scripts/ folder.
* tests go into the test/ folder.
* IMPORTANT : workflows go into .github/workflows , workflows must be used for testing, testing should be done using
github workflows , NOT locally , run github workflows remotely using "gh workflow run <NAME>" and "gh run list" and
"gh run view <ID>" to run workflows list them and check their status and logs via "run and gh run view <RUN_ID>". 
All automations including CI CD, tests of WSL on windows and dockup on windows and dockup tests must ALL be done using gh , 
the host machine shall NOT be used for anything besides seeing if dockup compiles properly. the only other thing that can be 
done on the host machine is run scripts\ops\go-bugcheck.sh for linting, so host machine can only ; 1-compile dockup for 
comilation tests, 2-run the bugcheck script, it shall NOT be used for running dockup commands like dockup etc. and the host 
machine system shall not be touched or modified, github actions already provides enough stuff to do testing etc. make sure to 
frequently check the gh actions logs though, do not blindly run the actions , see if they failed or not and see what the logs 
gave back, again gh run view <ID> to view run and gh run view <RUN_ID> --log for logs.

# A Quick Technical Note :

standard TCP/named-pipe connections from Windows CLI to a WSL Docker daemon often struggle with interactive flags (-i, -t, or terminal TTY resizing) unless specially handled.

Docker Desktop solves this using a custom Windows named pipe (\\.\pipe\docker_engine) that translates Windows console handles directly into the WSL socket via a companion helper process (docker-proxy / backend service). we will likely need a similar socket/stream-forwarding mechanism so container logs and interactive sessions (docker run -it) pipe cleanly back to the Windows command prompt.

#### To achieve seamless interactive sessions (-i, -t, and TTY resizing) without forcing users to rely on raw TCP ports, we can write a small companion helper binary (in Go, C#, or Rust) that runs on the Windows side.

Listen on Windows: Use a named pipe listener on Windows (e.g., \\.\pipe\dockup_engine or mirroring \\.\pipe\docker_engine). In Go, we can use the [github.com/Microsoft/go-winio](https://github.com/Microsoft/go-winio) package to easily set up a named pipe listener.

Connect to WSL: Forward incoming named pipe connections to the WSL distro. You can communicate with your Debian dockup distro either by:

Connecting directly to its internal Unix socket (/var/run/docker.sock) using WSL's interop socket sharing.

Or binding the daemon inside WSL to a local TCP port or a localized WSL IP address, and proxying the streams across.

Handle Bi-Directional Streams & TTY: Use standard stream copying (io.Copy in Go) to pipe stdin, stdout, and stderr transparently between the Windows client and the WSL backend. For TTY resizing, we will want to listen for API calls containing resize dimensions and forward those control sequences to the Docker API endpoint.

This approach keeps dockup entirely self-contained, open-source, and avoids any legal or technical entanglements with Docker Desktop's proprietary binaries.

#### note that this is up for discussion, it is also important to make this helper work based on how dockup is ran, if you run dockup as forground by simply running "dockup" , this helper must also be a forground and exit upon a ctrl + C on the dockup foreground process or upon a dockup shutdown, if dockup is ran as a daemon via the daemon command it shall start stop and restart etc. alongisde the daemon when the daemon is touched. the helper shouldnt be always running it should run alongside dockup however dockup is running , and it should obviously be respected and maintained by dockup itself so if the helper fails, dockup wont continue and will error out as well in order to prevent unwanted behaviour, for example if you run "dockup" it should start the helper too and show it starting up and tell us if its up and running, since the foreground already logs everything inline, and for daemon start stop and restart dockup should wait for the helper to start stop or restart, before assuring the user that dockup has started stopped or restarted scucessfully , again to ensure no strange stuff happens. and in case the helper does fail, the dockup foreground process, or the daemon background process, must exit and error out explaining that the helper failed , alongisde an explanation for why it failed (error code etc.)