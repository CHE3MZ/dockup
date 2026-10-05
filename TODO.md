- [ ] address these problems:

where do you want to install the dockup distro?
enter path e.g D:/WSL [default: C:/Users/USERNAME/AppData/Local/dockup/wsl]: D:/WSL/Dockup
installing debian... (0/46 mb)
installing debian... (46/46 mb)
importing debian into WSL as "dockup"... (0%)
downloaded rootfs-amd64-bookworm.tar.gz (46 MB) -> C:\Users\USERNAME\AppData\Local\Temp\dockup-rootfs-amd64.tar.gz
importing debian into WSL as "dockup"... (100%)
testing dockup on WSL... (please wait.)
test results : success
installing docker... (0%)
installing docker... (100%)
configuring docker...  (0%)
configuring docker...  (100%)
testing docker... (0%)
test results : success
testing the docker daemon bridge... (0%)
test results : success
you're all good to go ! run "dockup" to start a foreground process or "dockup daemon start" to start a background daemon process.

issue: enter path e.g is bad because it uses D:/WSL instead of D:/WSL/Dockup
issue: two installing debian... texts one has 0/46 the other moves the first one is static
issue: importing debian into wsl as dockup is stuck 0% in display
installing docker... is stuck at 0% the actual progress doesnt show, there is a duplicate test that shows it, configuring docker has the same issue
issue: everything with a progress basically either doesnt work, has a duplicate or both

other issues:

PS C:\Users\USERNAME> dockup
Starting dockup...
dockup is up on \\.\pipe\dockup_engine
use: docker -H npipe:////./pipe/dockup_engine version
Running in the foreground — press Ctrl+C to stop.

PS C:\Users\USERNAME> docker ps
failed to connect to the docker API at npipe:////./pipe/docker_engine; check if the path is correct and if the daemon is running: open //./pipe/docker_engine: The system cannot find the file specified.

PS C:\Users\USERNAME> dockup ps
STATUS     AUTOSTART    INSTALLED    SIZE      MEMORY
running    off          yes          1.0 GB    144 MB

PS C:\Users\USERNAME> docker ps
failed to connect to the docker API at npipe:////./pipe/docker_engine; check if the path is correct and if the daemon is running: open //./pipe/docker_engine: The system cannot find the file specified.

issues: dockup doesnt work, docker says no daemon, everything is just messy and aint working.   

issues: cannot CTRL + C foreground dockup it just doesnt work

other issues: the CLI looks horrible the text looks bad and unprofessional uninteractive and overall just garbage.