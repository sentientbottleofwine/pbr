package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"

	"pbr/notifications"

	"github.com/fsnotify/fsnotify"
	"github.com/go-git/go-git/v5"
	"github.com/shirou/gopsutil/disk"
	"golang.org/x/sync/errgroup"
)

type arguments struct {
	databasePath            string
	storageDeviceMountpoint string
	gitRemote               string
}

type argumentError struct {
	s string
}

func (e *argumentError) Error() string {
	return e.s
}

func checkPath(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

func parseArgs() (arguments, error) {
	var args arguments
	const argCount = 4
	const help = `pbr is a small program that automates and reminds you of backing up your passwords you silly goose
usage: pbr [database_path] [storage_device_mount_point] [git_remote]
	
In the database path there has to be a git repo that has a remote added to  it`
	if len(os.Args) < argCount {
		return args, &argumentError{"Database path, storage device mountpoint or path to git repo is not defined.\n" + help}
	} else if len(os.Args) > argCount {
		return args, &argumentError{"Too many arguments.\n" + help}
	}

	// Check db
	stat, err := os.Stat(os.Args[1])
	if errors.Is(err, os.ErrNotExist) || stat.IsDir() {
		return args, &argumentError{"Invalid path: " + os.Args[1]}
	} else if err != nil {
		return args, err
	}

	// Check mountpoint
	_, err = os.Stat(os.Args[2])
	if errors.Is(err, os.ErrNotExist) {
		return args, &argumentError{"Invalid path: " + os.Args[2]}
	} else if err != nil {
		return args, err
	}

	// Useful to later get a relative
	args.databasePath, err = filepath.Abs(os.Args[1])
	args.storageDeviceMountpoint = os.Args[2]
	args.gitRemote = os.Args[3]

	return args, err
}

func waitUntilWrite(databasePath string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	err = watcher.Add(databasePath)
	if err != nil {
		return err
	}

	eg := new(errgroup.Group)

	eg.Go(func() error {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return nil
				}

				/* By default when keepass saves, for example a password entry it first
				deletes the old db file and then moves the updated temp file into place.
				Checking for write in case direct write is configured. */
				if event.Has(fsnotify.Remove) || event.Has(fsnotify.Write) {
					log.Print("Write to database has been detected")
					return nil
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return nil
				}

				return err
			}
		}
	})

	return eg.Wait()
}

func comparePaths(lpath, rpath string) (bool, error) {
	lpath, err := filepath.Abs(lpath)
	if err != nil {
		return false, err
	}
	rpath, err = filepath.Abs(rpath)
	if err != nil {
		return false, err
	}

	lpath, err = filepath.EvalSymlinks(lpath)
	if err != nil {
		return false, err
	}
	rpath, err = filepath.EvalSymlinks(rpath)
	if err != nil {
		return false, err
	}

	return lpath == rpath, nil
}

func isMountpoint(mountpoint string) (bool, error) {
	partitions, err := disk.Partitions(false)
	if err != nil {
		return false, err
	}

	var mountpoints []string
	for _, partition := range partitions {
		mountpoints = append(mountpoints, partition.Mountpoint)
	}

	for _, mount := range mountpoints {
		equalPaths, err := comparePaths(mount, mountpoint)
		if err != nil {
			return false, err
		}

		if equalPaths {
			return true, nil
		}
	}

	return false, nil
}

func makeBackupPhys(args arguments) error {
	db, err := os.Open(args.databasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	var backedup_db *os.File
	backedup_db, err = os.Create(args.storageDeviceMountpoint + "/" + path.Base(args.databasePath))
	if err != nil {
		return err
	}
	defer backedup_db.Close()

	_, err = io.Copy(backedup_db, db)
	return err
}

func makeBackupGit(args arguments) error {
	const commitMessage = "Update to db"
	repoPath := filepath.Dir(args.databasePath)
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("Failed to open repo at: %s: %v", repoPath, err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}

	dbRelPath, err := filepath.Rel(repoPath, args.databasePath)
	if err != nil {
		return err
	}

	_, err = worktree.Add(dbRelPath)
	if err != nil {
		return fmt.Errorf("Failed to add: %s: %v", args.databasePath, err)
	}

	_, err = worktree.Commit(commitMessage, &git.CommitOptions{})
	if err != nil {
		return fmt.Errorf("Failed to commit: %v", err)
	}

	err = repo.Push(&git.PushOptions{
		RemoteName: args.gitRemote,
	})
	if err != nil {
		return fmt.Errorf("Failed to push: %v", err)
	}

	return nil
}

func gracefulErrorOnExit(err error) {
	if arg_err, ok := err.(*argumentError); ok {
		log.Fatal(arg_err)
		os.Exit(1)
	}

	notifications.Notify("pbr encounterred an error", err.Error(), 0)
	panic(err)
}

func main() {
	args, err := parseArgs()
	if err != nil {
		gracefulErrorOnExit(err)
	}

	for {
		err := waitUntilWrite(args.databasePath)
		if err != nil {
			gracefulErrorOnExit(err)
		}

		mountpointValid, err := isMountpoint(args.storageDeviceMountpoint)
		if err != nil {
			gracefulErrorOnExit(err)
		}

		if !mountpointValid {
			notify_until := notifications.NotifyUntilClosure()
			err = notify_until("NOT A VALID MOUNTPOINT", "Please either change the mountpoint or plug in and mount the backup drive", func() bool {
				mountpointValid, err := isMountpoint(args.storageDeviceMountpoint)
				if err != nil {
					gracefulErrorOnExit(err)
				}

				if mountpointValid {
					return true
				}
				return false
			})
		}

		notifications.Notify("Backing up the db", "Copying to backup drive.\nPushing changes to remote.", 0)
		err = makeBackupPhys(args)
		if err != nil {
			gracefulErrorOnExit(err)
		}

		err = makeBackupGit(args)
		if err != nil {
			gracefulErrorOnExit(err)
		}
	}
}
