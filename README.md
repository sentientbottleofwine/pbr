# pbr
A program that backs up your passwords, becuase you're to lazy

## Dependencies
```
notify-send
<whatever you're using to auth for git>
```

## Build
```sh
cd src/
go build ..
```
To install just:
```sh
go install .
```
## Usage
```
usage: pbr [database_path] [storage_device_mount_point] [git_remote]
	
In the database path there has to be a git repo that has a remote added to it. Git remote is often beign set as origin.
```
> [!TIP]
> If you're using ssh as auth remember to [configure](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/generating-a-new-ssh-key-and-adding-it-to-the-ssh-agent#adding-your-ssh-key-to-the-ssh-agent) your ssh agent correctly
