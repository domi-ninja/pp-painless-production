in a subfolder of this repo, make a cjs script

functionality:

- goes through folders inside of /root/repos 
- checks if upstream is git.domi.ninja, ssh forgejo key
- if not: 
  - renames current upstream to an name based on current upstream domain or whatever. if its a github.com -> github, etc
  - makes private repo if not exist on git.domi.ninja forgejo
  - adds that as a new ssh upstream
  - sets that upstream and pushes all branches to it
